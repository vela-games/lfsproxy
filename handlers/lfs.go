package handlers

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/allegro/bigcache/v3"
	"github.com/gin-gonic/gin"
	"github.com/vela-games/lfsproxy/cache"
	"github.com/vela-games/lfsproxy/config"
	"github.com/vela-games/lfsproxy/exporter"
	"github.com/vela-games/lfsproxy/services"
)

type LFSHandler struct {
	cache         cache.Cache
	promCollector *exporter.LFSProxyCollector
	awsService    services.AWSService
	config        *config.Config
}

func NewLFSHandler(ctx context.Context, cfg *config.Config) (*LFSHandler, error) {
	cache, err := cache.NewCache(ctx, cfg.CacheEviction)
	if err != nil {
		return nil, err
	}

	awsService, err := services.NewAWSService(cfg.S3Bucket, cfg.S3UseAccelerate, cfg.S3PresignEnabled, cfg.S3PresignExpiration)
	if err != nil {
		return nil, err
	}

	return &LFSHandler{
		cache:         cache,
		promCollector: exporter.NewCollector(),
		config:        cfg,
		awsService:    awsService,
	}, nil
}

func (l LFSHandler) PostBatch(c *gin.Context) {
	// Parse LFS Batch Request to Struct
	var batchRequest BatchRequest
	if err := c.ShouldBindJSON(&batchRequest); err != nil {
		c.AbortWithError(500, err) //nolint:errcheck
		return
	}

	// Create Modified Batch Request that will only contain objects to be requested to upstream
	// These would be the ones not cached in memory
	modifiedBatchRequest := BatchRequest{
		Operation: batchRequest.Operation,
		Transfers: batchRequest.Transfers,
		Ref:       batchRequest.Ref,
		HashAlgo:  batchRequest.HashAlgo,
		Objects:   []*BatchObjectResponse{},
	}

	// Contains a mix of cached and uncached objects
	finalBatchResponse := BatchResponse{
		Objects: []*BatchObjectResponse{},
	}

	// Check if any of the objects being requested is cached in-memory
	// If they are then don't include them on the modified batch request and add them to the final batch response
	for _, object := range batchRequest.Objects {
		data, err := l.cache.Get(object.OID)
		if err == nil {
			l.promCollector.CacheHits.Add(1)
			var cachedBatchObjectResponse BatchObjectResponse
			if err := json.Unmarshal(data, &cachedBatchObjectResponse); err == nil {
				if l.config.S3PresignEnabled {
					go l.checkCachedLink(object.OID, cachedBatchObjectResponse.Actions["download"].HeadHref)
				}
				finalBatchResponse.Objects = append(finalBatchResponse.Objects, &cachedBatchObjectResponse)
				continue
			}
		} else if errors.Is(err, bigcache.ErrEntryNotFound) {
			l.promCollector.CacheMiss.Add(1)
		}

		modifiedBatchRequest.Objects = append(modifiedBatchRequest.Objects, object)
	}

	// If we have objects to request to github because they were not cached
	if len(modifiedBatchRequest.Objects) > 0 {
		upstreamBatchResponse, statusCode, err := l.getFromUpstream(c, modifiedBatchRequest, c.Request.URL.Path, c.Request.Header)
		if err != nil {
			c.AbortWithError(statusCode, err) //nolint:errcheck
			return
		}

		finalBatchResponse.Transfer = upstreamBatchResponse.Transfer

		urls := make(chan BatchObjectResponse)

		totalUrls := len(upstreamBatchResponse.Objects)

		// For each of the objects returned by upstream
		// check if we have them on S3, if not return the upstream url
		for _, obj := range upstreamBatchResponse.Objects {
			_, ok := obj.Actions["download"]
			if !ok {
				totalUrls--
				continue
			}

			obj := obj

			go l.pullS3(*obj, urls)
		}

		count := 0
		for r := range urls {
			r := r
			finalBatchResponse.Objects = append(finalBatchResponse.Objects, &r)
			count++

			if count >= totalUrls {
				close(urls)
				break
			}
		}
	}

	c.JSON(200, finalBatchResponse)
}

func (l LFSHandler) getFromUpstream(ctx context.Context, batchRequest BatchRequest, urlPath string, headers http.Header) (*BatchResponse, int, error) {
	upstreamURL, err := url.Parse(l.config.UpstreamBaseURL)
	if err != nil {
		return nil, 500, err
	}

	var buf bytes.Buffer
	err = json.NewEncoder(&buf).Encode(batchRequest)
	if err != nil {
		return nil, 500, err
	}

	// Create new reverse proxy request
	req, err := http.NewRequestWithContext(ctx, "POST", upstreamURL.Path+strings.TrimLeft(urlPath, "/"), &buf)
	if err != nil {
		log.Printf("unexpected error creating request %v\n", err.Error())
		return nil, 500, err
	}

	req.Header = headers
	req.Host = upstreamURL.Host
	req.URL.Scheme = upstreamURL.Scheme
	req.URL.Host = upstreamURL.Host

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("unexpected error from upstream %v\n", err.Error())
		return nil, 500, err
	}

	if !resp.Uncompressed && strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		var err error
		if resp.Body, err = gzip.NewReader(resp.Body); err != nil {
			log.Printf("unexpected error uncompressing response %v\n", err.Error())
			return nil, 500, err
		}
	}

	if resp.StatusCode != 200 {
		respBytes, _ := io.ReadAll(resp.Body)
		return nil, resp.StatusCode, errors.New(string(respBytes))
	}

	// Parse Response to BatchResponse struct
	var upstreamBatchResponse BatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&upstreamBatchResponse); err != nil {
		return nil, 500, err
	}

	return &upstreamBatchResponse, resp.StatusCode, nil
}

func (l LFSHandler) pullS3(obj BatchObjectResponse, urls chan<- BatchObjectResponse) {
	batchResp := BatchObjectResponse{
		OID:           obj.OID,
		Size:          obj.Size,
		Authenticated: obj.Authenticated,
		Actions:       obj.Actions,
	}
	objectAction := obj.Actions["download"]

	exists, err := l.awsService.OIDExists(obj.OID)
	if err != nil {
		log.Printf("error: %v\n", err.Error())
		urls <- batchResp
		return
	}

	if exists {
		url, headUrl, err := l.awsService.GetOIDPreSignedURL(obj.OID)
		if err != nil {
			log.Printf("error presigned: %v\n", err.Error())
			urls <- batchResp
			return
		}

		objectAction.Href = url
		objectAction.HeadHref = headUrl

		batchResp.Actions["download"] = objectAction
		if err := l.cacheObjResponse(obj.OID, batchResp); err != nil {
			log.Printf("error caching response %v\n", err.Error())
		}

		l.promCollector.S3Hits.Add(1)
	} else {
		resp, err := http.Get(batchResp.Actions["download"].Href)
		if err == nil && resp.StatusCode == 200 {
			go l.pushToS3(obj, resp.Body)
		}
		l.promCollector.S3Miss.Add(1)
	}
	urls <- batchResp
}

func (l LFSHandler) pushToS3(obj BatchObjectResponse, body io.ReadCloser) {
	err := l.awsService.UploadOID(obj.OID, body)
	if err != nil {
		log.Printf("error uploading to S3: %v\n", err.Error())
		return
	}

	url, headUrl, err := l.awsService.GetOIDPreSignedURL(obj.OID)
	if err != nil {
		log.Printf("error getting presigned: %v\n", err.Error())
		return
	}

	cacheResp := BatchObjectResponse{
		OID:           obj.OID,
		Size:          obj.Size,
		Authenticated: obj.Authenticated,
		Actions: map[string]*BatchObjectActionResponse{
			"download": {
				Href:      url,
				HeadHref:  headUrl,
				Header:    obj.Actions["download"].Header,
				ExpiresIn: obj.Actions["download"].ExpiresIn,
				ExpiresAt: obj.Actions["download"].ExpiresAt,
			},
		},
	}

	if err := l.cacheObjResponse(obj.OID, cacheResp); err != nil {
		log.Printf("error pre-caching response %v\n", err.Error())
	}
}

func (l LFSHandler) checkCachedLink(oid string, headHref string) {
	r, _ := http.DefaultClient.Head(headHref)
	if r.StatusCode != 200 {
		log.Printf("removing %v from cache due to expired presigned link: %v\n", headHref, r.StatusCode)
		l.cache.Delete(oid) //nolint:errcheck
	}
}

func (l LFSHandler) cacheObjResponse(key string, obj BatchObjectResponse) error {
	var err error
	var data []byte

	if data, err = json.Marshal(obj); err == nil {
		err = l.cache.Set(key, data)
	}

	return err
}
