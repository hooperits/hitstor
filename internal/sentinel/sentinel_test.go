package sentinel

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestSentinelMockDetection_Safe(t *testing.T) {
	os.Setenv("HITSTOR_TEST", "true")
	defer os.Unsetenv("HITSTOR_TEST")

	s := New(Config{Enabled: true})

	current := TelemetryData{
		Time:           time.Now(),
		FilesNew:       2,
		FilesUnchanged: 98,
		BytesAdded:     1024,
		BytesProcessed: 102400, // compressed/dedup ratio: 100x
		Duration:       5.5,
	}

	analysis, err := s.CheckSession(context.Background(), "test-agent", current, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if analysis.ThreatDetected {
		t.Errorf("expected threat_detected to be false for normal telemetry, got true")
	}
	if analysis.Confidence >= 0.5 {
		t.Errorf("expected low confidence rating, got %f", analysis.Confidence)
	}
}

func TestSentinelMockDetection_Threat(t *testing.T) {
	os.Setenv("HITSTOR_TEST", "true")
	defer os.Unsetenv("HITSTOR_TEST")

	s := New(Config{Enabled: true})

	// Ransomware pattern: High FilesNew, 0 FilesUnchanged, collapse of compression ratio (BytesAdded == BytesProcessed)
	current := TelemetryData{
		Time:           time.Now(),
		FilesNew:       50,
		FilesUnchanged: 0,
		BytesAdded:     500000,
		BytesProcessed: 500000, // ratio: 1.0 (collapse)
		Duration:       2.1,
	}

	analysis, err := s.CheckSession(context.Background(), "test-agent", current, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !analysis.ThreatDetected {
		t.Errorf("expected threat_detected to be true for ransomware telemetry, got false")
	}
	if analysis.Confidence < 0.8 {
		t.Errorf("expected high confidence rating, got %f", analysis.Confidence)
	}
}

func TestSentinelBucketLocks_Mock(t *testing.T) {
	os.Setenv("HITSTOR_TEST", "true")
	defer os.Unsetenv("HITSTOR_TEST")

	t.Run("GCS", func(t *testing.T) {
		s := New(Config{Enabled: true, GCSBucket: "test-bucket", BackendType: "gcs"})
		err := s.LockGCSBucket(context.Background())
		if err != nil {
			t.Fatalf("expected no GCS lock error in test environment, got %v", err)
		}
	})

	t.Run("S3", func(t *testing.T) {
		s := New(Config{
			Enabled:     true,
			BackendType: "s3",
			S3Bucket:    "test-s3-bucket",
			S3Region:    "us-west-2",
		})
		err := s.LockGCSBucket(context.Background())
		if err != nil {
			t.Fatalf("expected no S3 lock error in test environment, got %v", err)
		}
	})

	t.Run("Azure", func(t *testing.T) {
		s := New(Config{
			Enabled:             true,
			BackendType:         "azure",
			AzureContainer:      "test-container",
			AzureStorageAccount: "testaccount",
			AzureTenant:         "tenant-id",
			AzureClient:         "client-id",
		})
		err := s.LockGCSBucket(context.Background())
		if err != nil {
			t.Fatalf("expected no Azure lock error in test environment, got %v", err)
		}
	})
}

func TestSentinelMockDetection_SuspiciousFiles(t *testing.T) {
	os.Setenv("HITSTOR_TEST", "true")
	defer os.Unsetenv("HITSTOR_TEST")

	s := New(Config{Enabled: true})

	current := TelemetryData{
		Time:            time.Now(),
		FilesNew:        20,
		FilesUnchanged:  50,
		BytesAdded:      1024,
		BytesProcessed:  10240,
		Duration:        3.4,
		SuspiciousFiles: 2,
	}

	analysis, err := s.CheckSession(context.Background(), "test-agent", current, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !analysis.ThreatDetected {
		t.Errorf("expected threat_detected to be true due to SuspiciousFiles > 0, got false")
	}
}
