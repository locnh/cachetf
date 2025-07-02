package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/sirupsen/logrus"
	"cachetf/internal/metrics"
)

// S3Storage implements Storage interface for S3
type S3Storage struct {
	client     *s3.Client
	bucket     string
	logger     *logrus.Logger
	uploader   *manager.Uploader
	downloader *manager.Downloader
	metrics    *metrics.CacheMetrics
}

// S3Config holds the configuration for S3 storage
type S3Config struct {
	Bucket string
	Region string
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
		metrics:    metrics.NewCacheMetrics(),
	}, nil
}

// Get downloads a file from S3
func (s *S3Storage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	// Check if file exists first
	exists, err := s.Exists(ctx, key)
	if err != nil {
		return nil, err
	}

	if !exists {
		s.metrics.RecordMiss()
		s.logger.WithField("key", key).Debug("Cache miss: file not found in S3")
		return nil, os.ErrNotExist
	}

	// File exists, get it
	input := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	result, err := s.client.GetObject(ctx, input)
	if err != nil {
		s.logger.WithError(err).WithField("key", key).Error("Failed to get object from S3")
		return nil, fmt.Errorf("failed to get object %s: %v", key, err)
	}

	// Record the hit and update metrics
	s.metrics.RecordHit()
	if result.ContentLength != nil {
		s.metrics.UpdateSize(*result.ContentLength)
	}

	s.logger.WithField("key", key).Debug("Cache hit: file found in S3")
	return result.Body, nil
}

// Put uploads a file to S3
func (s *S3Storage) Put(ctx context.Context, key string, data io.Reader) error {
	// Check if file already exists to update size metrics
	exists, err := s.Exists(ctx, key)
	if err != nil {
		s.logger.WithError(err).WithField("key", key).Error("Failed to check if object exists")
		return fmt.Errorf("failed to check if object exists: %w", err)
	}

	if exists {
		// Get the current size to update metrics
		head, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(key),
		})
		if err == nil && head.ContentLength != nil {
			s.metrics.UpdateSize(-*head.ContentLength)
		}
	}

	// Upload the file
	result, err := s.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   data,
	})

	if err != nil {
		s.metrics.RecordError("put")
		return fmt.Errorf("failed to upload object %s: %v", key, err)
	}

	// Update metrics with new size if available
	if result != nil && result.UploadID != "" {
		head, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(key),
		})
		if err == nil && head.ContentLength != nil {
			s.metrics.UpdateSize(*head.ContentLength)
		}
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
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return false, nil
		}
		s.metrics.RecordError("exists")
		return false, fmt.Errorf("failed to check if object exists: %w", err)
	}

	return true, nil
}

// DeleteByPrefix deletes all objects with the given prefix
func (s *S3Storage) DeleteByPrefix(ctx context.Context, prefix string) (int, error) {
	s.logger.WithField("prefix", prefix).Info("Deleting objects by prefix")
	
	// List all objects with the given prefix
	var objectIds []types.ObjectIdentifier
	var totalSize int64
	var continuationToken *string
	var deletedCount int

	// First, list all objects to get their sizes
	for {
		// List objects with pagination
		listInput := &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuationToken,
		}

		listOutput, err := s.client.ListObjectsV2(ctx, listInput)
		if err != nil {
			s.metrics.RecordError("delete_by_prefix")
			return deletedCount, fmt.Errorf("failed to list objects: %w", err)
		}

		// Add objects to delete batch and accumulate their sizes
		for _, obj := range listOutput.Contents {
			objectIds = append(objectIds, types.ObjectIdentifier{Key: obj.Key})
			if obj.Size != nil && *obj.Size > 0 {
				totalSize += *obj.Size
			}
		}

		// If there are no more objects, break the loop
		if !aws.ToBool(listOutput.IsTruncated) {
			break
		}
		continuationToken = listOutput.NextContinuationToken
	}

	// If no objects found, return early
	if len(objectIds) == 0 {
		s.logger.WithField("prefix", prefix).Info("No objects found with prefix")
		return 0, nil
	}

	// Delete objects in batches of 1000 (S3 API limit)
	for i := 0; i < len(objectIds); i += 1000 {
		end := i + 1000
		if end > len(objectIds) {
			end = len(objectIds)
		}

		batch := objectIds[i:end]
		_, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(s.bucket),
			Delete: &types.Delete{
				Objects: batch,
				Quiet:   aws.Bool(true),
			},
		})

		if err != nil {
			s.metrics.RecordError("delete_by_prefix")
			return deletedCount, fmt.Errorf("failed to delete objects: %w", err)
		}

		deletedCount += len(batch)
	}

	// Update metrics
	if totalSize > 0 {
		s.metrics.UpdateSize(-totalSize)
	}
	s.metrics.RecordDeletion(deletedCount)

	s.logger.WithFields(logrus.Fields{
		"prefix": prefix,
		"count":  deletedCount,
		"size":   totalSize,
	}).Info("Finished deleting objects by prefix")

	return deletedCount, nil
}
