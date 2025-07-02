package storage

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/sirupsen/logrus"
)

// S3Storage implements Storage interface for S3
type S3Storage struct {
	client     *s3.Client
	bucket     string
	logger     *logrus.Logger
	uploader   *manager.Uploader
	downloader *manager.Downloader
}

// S3Config holds the configuration for S3 storage
type S3Config struct {
	Bucket  string
	Region  string
	RoleARN string
}

// NewS3Storage creates a new S3 storage instance
func NewS3Storage(cfg *S3Config, logger *logrus.Logger) (*S3Storage, error) {
	if cfg == nil {
		return nil, errors.New("S3 config cannot be nil")
	}

	// Load the default AWS configuration
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// If a role ARN is provided, assume the role
	if cfg.RoleARN != "" {
		stsSvc := sts.NewFromConfig(awsCfg)
		creds := stscreds.NewAssumeRoleProvider(stsSvc, cfg.RoleARN)
		awsCfg.Credentials = aws.NewCredentialsCache(creds)
	}

	// Create an S3 client with the configuration
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.Retryer = retry.NewStandard(func(so *retry.StandardOptions) {
			so.MaxAttempts = 3
		})

		// Configure timeouts
		o.Retryer = retry.AddWithMaxAttempts(o.Retryer, 5)
	})

	uploader := manager.NewUploader(s3Client)
	downloader := manager.NewDownloader(s3Client)

	return &S3Storage{
		client:     s3Client,
		bucket:     cfg.Bucket,
		logger:     logger,
		uploader:   uploader,
		downloader: downloader,
	}, nil
}

// Get downloads a file from S3
func (s *S3Storage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	result, err := s.client.GetObject(ctx, input)
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, errors.New("object does not exist")
		}
		return nil, fmt.Errorf("failed to get object %s: %v", key, err)
	}

	return result.Body, nil
}

// Put uploads a file to S3
func (s *S3Storage) Put(ctx context.Context, key string, data io.Reader) error {
	_, err := s.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   data,
	})

	if err != nil {
		return fmt.Errorf("failed to upload object %s: %v", key, err)
	}

	s.logger.WithField("path", key).Info("Successfully uploaded object to S3")
	return nil
}

// Exists checks if a file exists in S3
func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var notFoundErr *types.NoSuchKey
		if errors.As(err, &notFoundErr) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check if object %s exists: %w", key, err)
	}
	return true, nil
}

