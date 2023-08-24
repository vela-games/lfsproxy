package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/gin-gonic/gin"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/vela-games/lfsproxy/config"
	"github.com/vela-games/lfsproxy/exporter"
)

type MockCache struct {
	Cache   map[string][]byte
	KeysHit *[]string
}

func (m MockCache) Get(key string) ([]byte, error) {
	data, ok := m.Cache[key]
	if !ok {
		return nil, errors.New("Entry not found")
	}

	*m.KeysHit = append(*m.KeysHit, key)

	return data, nil
}

func (m MockCache) Set(key string, entry []byte) error {
	m.Cache[key] = entry
	return nil
}

func (m MockCache) Delete(key string) error {
	delete(m.Cache, key)
	return nil
}

func (m MockCache) Reset() {
	m.Cache = make(map[string][]byte)
	m.KeysHit = &[]string{}
}

type MockAWSService struct {
	urls         map[string]string
	uploadCalled *bool
}

func (m MockAWSService) OIDExists(oid string) (bool, error) {
	_, ok := m.urls[oid]
	return ok, nil
}

func (m MockAWSService) GetOIDPreSignedURL(oid string) (string, string, error) {
	url := m.urls[oid]
	return url, url, nil
}

func (m MockAWSService) UploadOID(oid string, body io.ReadCloser) error {
	*m.uploadCalled = true
	return nil
}

func (m MockAWSService) Reset() {
	*m.uploadCalled = false
	m.urls = make(map[string]string)
}

func TestLFSHandler(t *testing.T) {
	cfg := &config.Config{
		UpstreamBaseURL: "https://fake-git-server.com/repository.git/",
		CacheEviction:   1 * time.Minute,
	}

	cache := MockCache{
		Cache:   make(map[string][]byte),
		KeysHit: &[]string{},
	}

	mockAWSService := MockAWSService{
		urls:         make(map[string]string),
		uploadCalled: aws.Bool(false),
	}

	lfsHandler := LFSHandler{
		cache:         cache,
		promCollector: exporter.NewCollector(),
		config:        cfg,
		awsService:    mockAWSService,
	}

	t.Run("it should get from upstream", func(t *testing.T) {
		defer cache.Reset()
		defer mockAWSService.Reset()

		httpmock.Activate()
		defer httpmock.DeactivateAndReset()

		httpmock.RegisterResponder("POST", "https://fake-git-server.com/repository.git/objects/batch",
			func(req *http.Request) (*http.Response, error) {
				resp, err := httpmock.NewJsonResponse(200, map[string]interface{}{
					"transfer": "basic",
					"objects": []map[string]interface{}{
						{
							"oid":           "1111111",
							"size":          123,
							"authenticated": true,
							"actions": map[string]interface{}{
								"download": map[string]interface{}{
									"href": "https://some-download.com",
									"header": map[string]interface{}{
										"Key": "value",
									},
									"expires_at": "2016-11-10T15:29:07Z",
								},
							},
						},
					},
					"hash_algo": "sha256",
				})
				return resp, err
			},
		)

		batchRequest := BatchRequest{
			Operation: "download",
			Transfers: []string{"basic"},
			Objects: []*BatchObjectResponse{
				{
					OID:  "123",
					Size: 123,
				},
				{
					OID:  "asd1234",
					Size: 123,
				},
			},
			Ref:      map[string]string{"name": "refs/heads/main"},
			HashAlgo: "sha256",
		}

		batchResponse, statusCode, err := lfsHandler.getFromUpstream(context.TODO(), batchRequest, "/objects/batch", http.Header{})
		assert.NoError(t, err)
		assert.Equal(t, 200, statusCode)
		assert.Equal(t, "basic", batchResponse.Transfer)
	})

	t.Run("it should return all cached responses", func(t *testing.T) {
		defer cache.Reset()
		defer mockAWSService.Reset()

		httpmock.Activate()
		defer httpmock.DeactivateAndReset()

		httpmock.RegisterResponder("POST", "https://fake-git-server.com/repository.git/objects/batch",
			func(req *http.Request) (*http.Response, error) {
				assert.FailNow(t, "should not call upstream")
				return nil, nil
			},
		)

		now := time.Now()
		obj := BatchObjectResponse{
			OID:           "123",
			Size:          123,
			Authenticated: false,
			Actions: map[string]*BatchObjectActionResponse{
				"download": {
					Href:     "https://fake-url.com",
					HeadHref: "https://fake-url.com",
					Header: map[string]string{
						"Content-Type": "application/octet-stream",
					},
					ExpiresIn: 0,
					ExpiresAt: now,
				},
			},
		}

		if data, err := json.Marshal(obj); err == nil {
			cache.Set("123", data)
		}

		w := httptest.NewRecorder()
		c, r := gin.CreateTestContext(w)

		r.POST("/objects/batch", lfsHandler.PostBatch)

		var jsonData = []byte(`{
			"operation": "download",
			"transfers": [ "basic" ],
			"ref": { "name": "refs/heads/main" },
			"objects": [
				{
					"oid": "123",
					"size": 123
				}
			],
			"hash_algo": "sha256"
		}`)

		var err error

		c.Request, err = http.NewRequest("POST", "http://localhost:9999/objects/batch", bytes.NewBuffer(jsonData))
		assert.NoError(t, err)

		c.Request.Header.Set("Content-Type", "application/vnd.git-lfs+json")

		r.ServeHTTP(w, c.Request)

		b, err := io.ReadAll(w.Body)
		assert.NoError(t, err)

		assert.Equal(t, 200, w.Code)
		assert.Equal(t, 1, len(*cache.KeysHit))

		expected := fmt.Sprintf(`{"objects":[{"oid":"123","size":123,"actions":{"download":{"href":"https://fake-url.com","head_href":"https://fake-url.com","header":{"Content-Type":"application/octet-stream"},"expires_at":"%v"}}}]}`, now.Format(time.RFC3339Nano))

		assert.Equal(t, expected, string(b))
	})

	t.Run("it should return a mix of cached and upstream responses - with no URLs from S3", func(t *testing.T) {
		defer cache.Reset()
		defer mockAWSService.Reset()

		httpmock.Activate()
		defer httpmock.DeactivateAndReset()

		httpmock.RegisterResponder("POST", "https://fake-git-server.com/repository.git/objects/batch",
			func(req *http.Request) (*http.Response, error) {
				resp, err := httpmock.NewJsonResponse(200, map[string]interface{}{
					"transfer": "basic",
					"objects": []map[string]interface{}{
						{
							"oid":           "1234",
							"size":          123,
							"authenticated": true,
							"actions": map[string]interface{}{
								"download": map[string]interface{}{
									"href": "https://some-download.com",
									"header": map[string]interface{}{
										"Key": "value",
									},
									"expires_at": "2016-11-10T15:29:07Z",
								},
							},
						},
					},
					"hash_algo": "sha256",
				})
				return resp, err
			},
		)

		httpmock.RegisterResponder("GET", "https://some-download.com", httpmock.NewStringResponder(200, ""))

		now := time.Now()
		obj := BatchObjectResponse{
			OID:           "123",
			Size:          123,
			Authenticated: false,
			Actions: map[string]*BatchObjectActionResponse{
				"download": {
					Href:     "https://fake-url.com",
					HeadHref: "https://fake-url.com",
					Header: map[string]string{
						"Content-Type": "application/octet-stream",
					},
					ExpiresIn: 0,
					ExpiresAt: now,
				},
			},
		}

		if data, err := json.Marshal(obj); err == nil {
			cache.Set("123", data)
		}

		w := httptest.NewRecorder()
		c, r := gin.CreateTestContext(w)

		r.POST("/objects/batch", lfsHandler.PostBatch)

		var jsonData = []byte(`{
			"operation": "download",
			"transfers": [ "basic" ],
			"ref": { "name": "refs/heads/main" },
			"objects": [
				{
					"oid": "123",
					"size": 123
				},
				{
					"oid": "1234",
					"size": 123
				}
			],
			"hash_algo": "sha256"
		}`)

		var err error

		c.Request, err = http.NewRequest("POST", "http://localhost:9999/objects/batch", bytes.NewBuffer(jsonData))
		assert.NoError(t, err)

		c.Request.Header.Set("Content-Type", "application/vnd.git-lfs+json")

		r.ServeHTTP(w, c.Request)

		b, err := io.ReadAll(w.Body)
		assert.NoError(t, err)

		assert.Equal(t, 200, w.Code)
		assert.Equal(t, 1, len(*cache.KeysHit))

		expected := fmt.Sprintf(`{"transfer":"basic","objects":[{"oid":"123","size":123,"actions":{"download":{"href":"https://fake-url.com","head_href":"https://fake-url.com","header":{"Content-Type":"application/octet-stream"},"expires_at":"%v"}}},{"oid":"1234","size":123,"authenticated":true,"actions":{"download":{"href":"https://some-download.com","header":{"Key":"value"},"expires_at":"2016-11-10T15:29:07Z"}}}]}`, now.Format(time.RFC3339Nano))

		assert.Equal(t, expected, string(b))

		assert.Eventually(t, func() bool {
			return *mockAWSService.uploadCalled
		}, 1*time.Second, 100*time.Millisecond)
	})

	t.Run("it should return a mix of cached and upstream responses - with URLs from S3", func(t *testing.T) {
		defer cache.Reset()
		defer mockAWSService.Reset()

		mockAWSService.urls["1234"] = "https://this-is-from-s3.com"

		httpmock.Activate()
		defer httpmock.DeactivateAndReset()

		httpmock.RegisterResponder("POST", "https://fake-git-server.com/repository.git/objects/batch",
			func(req *http.Request) (*http.Response, error) {
				resp, err := httpmock.NewJsonResponse(200, map[string]interface{}{
					"transfer": "basic",
					"objects": []map[string]interface{}{
						{
							"oid":           "1234",
							"size":          123,
							"authenticated": true,
							"actions": map[string]interface{}{
								"download": map[string]interface{}{
									"href": "https://some-download.com",
									"header": map[string]interface{}{
										"Key": "value",
									},
									"expires_at": "2016-11-10T15:29:07Z",
								},
							},
						},
					},
					"hash_algo": "sha256",
				})
				return resp, err
			},
		)

		now := time.Now()
		obj := BatchObjectResponse{
			OID:           "123",
			Size:          123,
			Authenticated: false,
			Actions: map[string]*BatchObjectActionResponse{
				"download": {
					Href:     "https://fake-url.com",
					HeadHref: "https://fake-url.com",
					Header: map[string]string{
						"Content-Type": "application/octet-stream",
					},
					ExpiresIn: 0,
					ExpiresAt: now,
				},
			},
		}

		if data, err := json.Marshal(obj); err == nil {
			cache.Set("123", data)
		}

		w := httptest.NewRecorder()
		c, r := gin.CreateTestContext(w)

		r.POST("/objects/batch", lfsHandler.PostBatch)

		var jsonData = []byte(`{
			"operation": "download",
			"transfers": [ "basic" ],
			"ref": { "name": "refs/heads/main" },
			"objects": [
				{
					"oid": "123",
					"size": 123
				},
				{
					"oid": "1234",
					"size": 123
				}
			],
			"hash_algo": "sha256"
		}`)

		var err error

		c.Request, err = http.NewRequest("POST", "http://localhost:9999/objects/batch", bytes.NewBuffer(jsonData))
		assert.NoError(t, err)

		c.Request.Header.Set("Content-Type", "application/vnd.git-lfs+json")

		r.ServeHTTP(w, c.Request)

		b, err := io.ReadAll(w.Body)
		assert.NoError(t, err)

		assert.Equal(t, 200, w.Code)
		assert.Equal(t, 1, len(*cache.KeysHit))

		expected := fmt.Sprintf(`{"transfer":"basic","objects":[{"oid":"123","size":123,"actions":{"download":{"href":"https://fake-url.com","head_href":"https://fake-url.com","header":{"Content-Type":"application/octet-stream"},"expires_at":"%v"}}},{"oid":"1234","size":123,"authenticated":true,"actions":{"download":{"href":"https://this-is-from-s3.com","head_href":"https://this-is-from-s3.com","header":{"Key":"value"},"expires_at":"2016-11-10T15:29:07Z"}}}]}`, now.Format(time.RFC3339Nano))

		assert.Equal(t, expected, string(b))

		assert.Equal(t, false, mockAWSService.uploadCalled)
	})
}
