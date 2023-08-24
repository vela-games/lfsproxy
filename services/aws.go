package services

import (
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type S3 interface {
	HeadObject(input *s3.HeadObjectInput) (*s3.HeadObjectOutput, error)
	HeadObjectRequest(input *s3.HeadObjectInput) (req *request.Request, output *s3.HeadObjectOutput)
	GetObjectRequest(input *s3.GetObjectInput) (req *request.Request, output *s3.GetObjectOutput)
}

type AWSService interface {
	OIDExists(oid string) (bool, error)
	GetOIDPreSignedURL(oid string) (string, string, error)
	UploadOID(oid string, body io.ReadCloser) error
}

type AWS struct {
	bucket            string
	useAccelerate     bool
	presignEnabled    bool
	presignExpiration time.Duration
	s3Client          S3
	awsSession        *session.Session
	awsRegion         string
}

func NewAWSService(bucket string, useAccelerate bool, presignEnabled bool, presignExpiration time.Duration) (AWSService, error) {
	session, err := GetAWSSession()
	if err != nil {
		return nil, err
	}

	s3Client := s3.New(session, &aws.Config{
		DisableRestProtocolURICleaning: aws.Bool(true),
		S3UseAccelerate:                aws.Bool(useAccelerate),
	})

	return &AWS{
		awsSession:        session,
		bucket:            bucket,
		useAccelerate:     useAccelerate,
		presignEnabled:    presignEnabled,
		presignExpiration: presignExpiration,
		s3Client:          s3Client,
		awsRegion:         *session.Config.Region,
	}, nil
}

func GetAWSSession() (*session.Session, error) {
	sess, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})

	if err != nil {
		return nil, err
	}

	return sess, nil
}

func (a AWS) OIDExists(oid string) (bool, error) {
	_, err := a.s3Client.HeadObject(&s3.HeadObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(oid),
	})

	if err != nil {
		if aerr, ok := err.(awserr.Error); ok { //nolint:errorlint
			switch aerr.Code() {
			case "NotFound":
				return false, nil
			}
		}

		return false, err
	}

	return true, nil
}

func (a AWS) GetOIDPreSignedURL(oid string) (string, string, error) {
	var urlStr, headUrlStr string

	if a.presignEnabled {
		var err error

		req, _ := a.s3Client.GetObjectRequest(&s3.GetObjectInput{
			Bucket: aws.String(a.bucket),
			Key:    aws.String(oid),
		})

		urlStr, err = req.Presign(a.presignExpiration)

		if err != nil {
			return "", "", err
		}

		req, _ = a.s3Client.HeadObjectRequest(&s3.HeadObjectInput{
			Bucket: aws.String(a.bucket),
			Key:    aws.String(oid),
		})

		headUrlStr, err = req.Presign(a.presignExpiration)
		if err != nil {
			return "", "", err
		}
	} else if a.useAccelerate {
		urlStr = fmt.Sprintf("https://%s.s3-accelerate.amazonaws.com/%s", a.bucket, oid)
		headUrlStr = urlStr
	} else {
		urlStr = fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", a.bucket, a.awsRegion, oid)
		headUrlStr = urlStr
	}

	return urlStr, headUrlStr, nil
}

func (a AWS) UploadOID(oid string, body io.ReadCloser) error {
	defer body.Close()

	uploader := s3manager.NewUploader(a.awsSession)

	_, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(oid),
		Body:   body,
	})
	if err != nil {
		fmt.Printf("error uploading: %v\n", err.Error())
		return err
	}

	return nil
}
