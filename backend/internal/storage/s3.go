package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Storage struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

// NewS3FromEnv builds a client from the standard AWS environment variables.
//
// LoadDefaultConfig reads AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY,
// AWS_DEFAULT_REGION and AWS_ENDPOINT_URL natively, so the Railway Bucket
// service variables are picked up as-is with no manual endpoint wiring. Only the
// bucket name is not an SDK variable, so we read that one ourselves.
//
// usePathStyle covers endpoints that reject virtual-hosted-style addressing.
func NewS3FromEnv(ctx context.Context, bucket string, usePathStyle bool) (*S3Storage, error) {
	if bucket == "" {
		return nil, fmt.Errorf("bucket name is empty (set AWS_S3_BUCKET_NAME)")
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = usePathStyle
	})

	return &S3Storage{
		client:  client,
		presign: s3.NewPresignClient(client),
		bucket:  bucket,
	}, nil
}

func (s *S3Storage) Put(ctx context.Context, key string, body io.Reader, contentType string, size int64) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(size),
	})
	if err != nil {
		return fmt.Errorf("put %s: %w", key, err)
	}
	return nil
}

func (s *S3Storage) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return s.presignGet(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, ttl)
}

func (s *S3Storage) PresignDownload(ctx context.Context, key, filename string, ttl time.Duration) (string, error) {
	return s.presignGet(ctx, &s3.GetObjectInput{
		Bucket:                     aws.String(s.bucket),
		Key:                        aws.String(key),
		ResponseContentDisposition: aws.String(contentDisposition(filename)),
	}, ttl)
}

func (s *S3Storage) presignGet(ctx context.Context, in *s3.GetObjectInput, ttl time.Duration) (string, error) {
	req, err := s.presign.PresignGetObject(ctx, in, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("presign %s: %w", aws.ToString(in.Key), err)
	}
	return req.URL, nil
}
