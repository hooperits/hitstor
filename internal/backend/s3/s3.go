package s3

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/hooperits/hitstor/internal/backend"
)

// S3Backend implements the backend.Backend interface using an S3-compatible
// object store (e.g. AWS S3, MinIO).
type S3Backend struct {
	client *s3.Client
	bucket string
}

// Compile-time check that S3Backend implements backend.Backend.
var _ backend.Backend = (*S3Backend)(nil)

// New creates a new S3 backend connected to the given endpoint.
// It uses path-style addressing for MinIO compatibility and static credentials.
func New(endpoint, bucket, accessKey, secretKey, region string) (*S3Backend, error) {
	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(endpoint),
		Region:       region,
		Credentials:  credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		UsePathStyle: true,
	})

	return &S3Backend{
		client: client,
		bucket: bucket,
	}, nil
}

// key returns the S3 object key for a given file type and ID.
func (b *S3Backend) key(ft backend.FileType, id backend.ID) string {
	idHex := id.String()
	switch ft {
	case backend.DataFile:
		return fmt.Sprintf("data/%s/%s", idHex[:2], idHex)
	case backend.ConfigFile:
		return "config"
	default:
		return fmt.Sprintf("%s/%s", ft.String(), idHex)
	}
}

// Save stores data from rd under the given file type and ID.
func (b *S3Backend) Save(ctx context.Context, ft backend.FileType, id backend.ID, rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return fmt.Errorf("read data: %w", err)
	}

	k := b.key(ft, id)
	_, err = b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(k),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		return fmt.Errorf("put object %s: %w", k, err)
	}

	return nil
}

// Load retrieves the data for the given file type and ID.
// The caller must close the returned ReadCloser.
func (b *S3Backend) Load(ctx context.Context, ft backend.FileType, id backend.ID) (io.ReadCloser, error) {
	k := b.key(ft, id)
	output, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(k),
	})
	if err != nil {
		return nil, fmt.Errorf("get object %s: %w", k, err)
	}

	return output.Body, nil
}

// Stat returns metadata about the file for the given file type and ID.
func (b *S3Backend) Stat(ctx context.Context, ft backend.FileType, id backend.ID) (backend.FileInfo, error) {
	k := b.key(ft, id)
	output, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(k),
	})
	if err != nil {
		return backend.FileInfo{}, fmt.Errorf("head object %s: %w", k, err)
	}

	return backend.FileInfo{Size: *output.ContentLength}, nil
}

// Remove deletes the file for the given file type and ID.
func (b *S3Backend) Remove(ctx context.Context, ft backend.FileType, id backend.ID) error {
	k := b.key(ft, id)
	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(k),
	})
	if err != nil {
		return fmt.Errorf("delete object %s: %w", k, err)
	}

	return nil
}

// List returns all IDs stored for the given file type.
func (b *S3Backend) List(ctx context.Context, ft backend.FileType) ([]backend.ID, error) {
	if ft == backend.ConfigFile {
		return nil, nil
	}

	prefix := ft.String() + "/"
	var ids []backend.ID

	paginator := s3.NewListObjectsV2Paginator(b.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(b.bucket),
		Prefix: aws.String(prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list objects with prefix %s: %w", prefix, err)
		}

		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			// Extract the filename (last component) from the key.
			parts := strings.Split(key, "/")
			name := parts[len(parts)-1]

			id, err := parseID(name)
			if err != nil {
				continue // skip objects with non-hex names
			}
			ids = append(ids, id)
		}
	}

	return ids, nil
}

// Close is a no-op for the S3 backend. The S3 client does not hold
// resources that need explicit cleanup.
func (b *S3Backend) Close() error {
	return nil
}

// HasRepoConfig reports whether the repository config object exists in the bucket.
func (b *S3Backend) HasRepoConfig(ctx context.Context) (bool, error) {
	_, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String("config"),
	})
	if err != nil {
		return false, nil
	}
	return true, nil
}

// TestConnection verifies that the S3 endpoint is reachable and the
// credentials can access the configured bucket via HeadBucket.
func (b *S3Backend) TestConnection(ctx context.Context) error {
	_, err := b.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(b.bucket),
	})
	if err != nil {
		return fmt.Errorf("s3: test connection: %w", err)
	}
	return nil
}

// Client returns the underlying S3 client for advanced operations
// (e.g. creating buckets in tests).
func (b *S3Backend) Client() *s3.Client {
	return b.client
}

// Bucket returns the bucket name.
func (b *S3Backend) Bucket() string {
	return b.bucket
}

// parseID parses a hex-encoded string into a backend.ID.
func parseID(s string) (backend.ID, error) {
	var id backend.ID

	raw, err := hex.DecodeString(s)
	if err != nil {
		return id, err
	}

	if len(raw) != len(id) {
		return id, fmt.Errorf("invalid ID length: got %d, want %d", len(raw), len(id))
	}

	copy(id[:], raw)
	return id, nil
}
