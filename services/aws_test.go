package services

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/client/metadata"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/stretchr/testify/assert"
)

type MockS3Client struct {
	bucket          string
	objectsInBucket []string
	beforePresign   func(r *request.Request) error
}

func (m MockS3Client) HeadObject(input *s3.HeadObjectInput) (*s3.HeadObjectOutput, error) {
	if *input.Bucket == m.bucket {
		for _, object := range m.objectsInBucket {
			if object == *input.Key {
				return &s3.HeadObjectOutput{}, nil
			}
		}
	}

	err := awserr.New("NotFound", "Object not found", nil)

	return nil, err
}

func (m MockS3Client) HeadObjectRequest(input *s3.HeadObjectInput) (req *request.Request, output *s3.HeadObjectOutput) {
	op := &request.Operation{
		Name:       "HeadObject",
		HTTPMethod: "HEAD",
		HTTPPath:   "/{Bucket}/{Key+}",
	}

	op.BeforePresignFn = m.beforePresign

	output = &s3.HeadObjectOutput{}
	req = request.New(*aws.NewConfig(), metadata.ClientInfo{}, request.Handlers{}, nil, op, input, output)
	return
}

func (m MockS3Client) GetObjectRequest(input *s3.GetObjectInput) (req *request.Request, output *s3.GetObjectOutput) {
	op := &request.Operation{
		Name:       "GetObject",
		HTTPMethod: "GET",
		HTTPPath:   "/{Bucket}/{Key+}",
	}

	op.BeforePresignFn = m.beforePresign

	output = &s3.GetObjectOutput{}
	req = request.New(*aws.NewConfig(), metadata.ClientInfo{}, request.Handlers{}, nil, op, input, output)
	return
}

func TestOIDExists(t *testing.T) {
	t.Run("OIDExists return false because OID doesn't exist", func(t *testing.T) {
		mockS3Client := MockS3Client{
			bucket:          "test-bucket",
			objectsInBucket: []string{},
		}

		awsService := AWS{
			bucket:   "test-bucket",
			s3Client: mockS3Client,
		}

		exists, err := awsService.OIDExists("test-oid")
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("OIDExists return true because OID exists", func(t *testing.T) {
		mockS3Client := MockS3Client{
			bucket: "test-bucket",
			objectsInBucket: []string{
				"test-oid",
			},
		}

		awsService := AWS{
			bucket:   "test-bucket",
			s3Client: mockS3Client,
		}

		exists, err := awsService.OIDExists("test-oid")
		assert.NoError(t, err)
		assert.True(t, exists)
	})
}

func TestGetOIDPreSignedURL(t *testing.T) {
	t.Run("Returns non-presign urls", func(t *testing.T) {
		mockS3Client := MockS3Client{
			bucket:          "test-bucket",
			objectsInBucket: []string{},
			beforePresign:   itShouldNotPresign(t),
		}

		awsService := AWS{
			bucket:         "test-bucket",
			s3Client:       mockS3Client,
			presignEnabled: false,
			useAccelerate:  false,
			awsRegion:      "eu-west-1",
		}

		urlStr, headUrlStr, err := awsService.GetOIDPreSignedURL("test-oid")
		assert.NoError(t, err)
		assert.Equal(t, "https://test-bucket.s3.eu-west-1.amazonaws.com/test-oid", urlStr)
		assert.Equal(t, "https://test-bucket.s3.eu-west-1.amazonaws.com/test-oid", headUrlStr)
	})

	t.Run("Returns non-presign s3 accelerate urls", func(t *testing.T) {
		mockS3Client := MockS3Client{
			bucket:          "test-bucket",
			objectsInBucket: []string{},
			beforePresign:   itShouldNotPresign(t),
		}

		awsService := AWS{
			bucket:         "test-bucket",
			s3Client:       mockS3Client,
			presignEnabled: false,
			useAccelerate:  true,
			awsRegion:      "eu-west-1",
		}

		urlStr, headUrlStr, err := awsService.GetOIDPreSignedURL("test-oid")
		assert.NoError(t, err)
		assert.Equal(t, "https://test-bucket.s3-accelerate.amazonaws.com/test-oid", urlStr)
		assert.Equal(t, "https://test-bucket.s3-accelerate.amazonaws.com/test-oid", headUrlStr)
	})

	t.Run("Returns presign s3 urls", func(t *testing.T) {
		var counter *int = aws.Int(0)
		mockS3Client := MockS3Client{
			bucket:          "test-bucket",
			objectsInBucket: []string{},
			beforePresign:   itShouldPresign(t, counter),
		}

		awsService := AWS{
			bucket:            "test-bucket",
			s3Client:          mockS3Client,
			presignEnabled:    true,
			useAccelerate:     false,
			presignExpiration: 1 * time.Hour,
			awsRegion:         "eu-west-1",
		}

		_, _, err := awsService.GetOIDPreSignedURL("test-oid")
		assert.NoError(t, err)
		assert.Equal(t, 2, *counter)
		//assert.Equal(t, "https://test-bucket.s3.eu-west-1.amazonaws.com/test-oid", urlStr)
		//assert.Equal(t, "https://test-bucket.s3.eu-west-1.amazonaws.com/test-oid", headUrlStr)

	})
}

// Not sure how to mock presign of URLs so we just take advantage of BeforePresignFn to check if
// Sign is being called or not
func itShouldNotPresign(t *testing.T) func(r *request.Request) error {
	return func(r *request.Request) error {
		assert.FailNow(t, "should not presign")
		return nil
	}
}

func itShouldPresign(t *testing.T, counter *int) func(r *request.Request) error {
	return func(r *request.Request) error {
		*counter++
		return nil
	}
}
