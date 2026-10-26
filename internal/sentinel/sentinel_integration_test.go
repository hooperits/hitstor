//go:build integration

package sentinel

import (
	"context"
	"net"
	"os"
	"testing"
	"time"
)

func skipIfNoMetadataServer(t *testing.T) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", "metadata.google.internal:80", 1*time.Second)
	if err != nil {
		t.Skip("GCP metadata server not available, skipping cloud integration tests")
	}
	conn.Close()
}

func TestSentinelGCSLock_Integration(t *testing.T) {
	skipIfNoMetadataServer(t)

	bucket := os.Getenv("HITSTOR_GCS_BUCKET")
	if bucket == "" {
		t.Skip("HITSTOR_GCS_BUCKET not set, skipping GCS Lock integration test")
	}

	s := New(Config{
		Enabled:       true,
		GCSBucket:     bucket,
		BackendType:   "gcs",
		RetentionDays: 1,
	})

	err := s.LockGCSBucket(context.Background())
	if err != nil {
		t.Fatalf("GCS lock policy patch failed: %v", err)
	}
}

func TestSentinelS3Lock_Integration(t *testing.T) {
	skipIfNoMetadataServer(t)

	bucket := os.Getenv("HITSTOR_S3_BUCKET")
	roleARN := os.Getenv("HITSTOR_AWS_ROLE_ARN")
	region := os.Getenv("HITSTOR_S3_REGION")
	if bucket == "" || roleARN == "" {
		t.Skip("HITSTOR_S3_BUCKET or HITSTOR_AWS_ROLE_ARN not set, skipping AWS S3 Object Lock integration test")
	}
	if region == "" {
		region = "us-east-1"
	}

	s := New(Config{
		Enabled:       true,
		BackendType:   "s3",
		S3Bucket:      bucket,
		S3Region:      region,
		S3RoleARN:     roleARN,
		RetentionDays: 1,
	})

	err := s.LockGCSBucket(context.Background())
	if err != nil {
		t.Fatalf("S3 lock configuration failed: %v", err)
	}
}

func TestSentinelAzureLock_Integration(t *testing.T) {
	skipIfNoMetadataServer(t)

	container := os.Getenv("HITSTOR_AZURE_CONTAINER")
	account := os.Getenv("HITSTOR_AZURE_STORAGE_ACCOUNT")
	tenantID := os.Getenv("HITSTOR_AZURE_TENANT")
	clientID := os.Getenv("HITSTOR_AZURE_CLIENT")

	if container == "" || account == "" || tenantID == "" || clientID == "" {
		t.Skip("Azure Storage environment variables not fully set, skipping Azure Lock policy integration test")
	}

	s := New(Config{
		Enabled:             true,
		BackendType:         "azure",
		AzureContainer:      container,
		AzureStorageAccount: account,
		AzureTenant:         tenantID,
		AzureClient:         clientID,
		RetentionDays:       1,
	})

	err := s.LockGCSBucket(context.Background())
	if err != nil {
		t.Fatalf("Azure container immutability policy failed: %v", err)
	}
}
