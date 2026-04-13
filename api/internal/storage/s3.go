package storage

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Client struct {
	client *minio.Client
	bucket string
}

func NewS3Client(endpoint, region, bucket, accessKey, secretKey string, timeout time.Duration) (*S3Client, error) {
	parsedEndpoint, err := url.Parse(strings.TrimRight(endpoint, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse s3 endpoint: %w", err)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}).DialContext
	transport.ResponseHeaderTimeout = timeout
	transport.TLSHandshakeTimeout = timeout

	client, err := minio.New(parsedEndpoint.Host, &minio.Options{
		Creds:     credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:    parsedEndpoint.Scheme == "https",
		Region:    region,
		Transport: transport,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	return &S3Client{
		client: client,
		bucket: bucket,
	}, nil
}

func (c *S3Client) HeadObject(ctx context.Context, key string) (bool, error) {
	_, err := c.client.StatObject(ctx, c.bucket, key, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}

	responseErr := minio.ToErrorResponse(err)
	if responseErr.StatusCode == 404 || responseErr.Code == "NoSuchKey" || responseErr.Code == "NoSuchObject" {
		return false, nil
	}

	return false, fmt.Errorf("head object %q: %w", key, err)
}

func (c *S3Client) PutObject(ctx context.Context, key string, body []byte, contentType string, extraHeaders map[string]string) error {
	opts := minio.PutObjectOptions{
		ContentType:  contentType,
		UserMetadata: map[string]string{},
	}

	for name, value := range extraHeaders {
		lowerName := strings.ToLower(name)
		switch lowerName {
		case "cache-control":
			opts.CacheControl = value
		case "content-disposition":
			opts.ContentDisposition = value
		default:
			opts.UserMetadata[name] = value
		}
	}

	_, err := c.client.PutObject(ctx, c.bucket, key, bytes.NewReader(body), int64(len(body)), opts)
	if err != nil {
		return fmt.Errorf("put object %q: %w", key, err)
	}

	return nil
}
