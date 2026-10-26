//go:build integration

package pruner

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/hooperits/hitstor/internal/archiver"
	s3be "github.com/hooperits/hitstor/internal/backend/s3"
	"github.com/hooperits/hitstor/internal/checker"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/restorer"
	"github.com/hooperits/hitstor/internal/retention"
)

// skipIfNoMinIO skips the test if MinIO is not reachable at localhost:9000.
func skipIfNoMinIO(t *testing.T) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", "localhost:9000", 2*time.Second)
	if err != nil {
		t.Skip("MinIO not available at localhost:9000, skipping integration tests")
	}
	conn.Close()
}

// setupTestBucketS3 creates a unique S3 bucket for testing and registers
// cleanup to delete all objects and the bucket when the test finishes.
func setupTestBucketS3(t *testing.T) (*s3be.S3Backend, string) {
	t.Helper()

	bucket := fmt.Sprintf("hitstor-prune-test-%d", time.Now().UnixNano())
	be, err := s3be.New("http://localhost:9000", bucket, "minioadmin", "minioadmin", "us-east-1")
	if err != nil {
		t.Fatalf("s3be.New: %v", err)
	}

	_, err = be.Client().CreateBucket(context.Background(), &awss3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		t.Fatalf("CreateBucket(%s): %v", bucket, err)
	}

	t.Cleanup(func() {
		ctx := context.Background()

		paginator := awss3.NewListObjectsV2Paginator(be.Client(), &awss3.ListObjectsV2Input{
			Bucket: aws.String(bucket),
		})
		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				t.Logf("cleanup: list objects: %v", err)
				break
			}
			for _, obj := range page.Contents {
				_, err := be.Client().DeleteObject(ctx, &awss3.DeleteObjectInput{
					Bucket: aws.String(bucket),
					Key:    obj.Key,
				})
				if err != nil {
					t.Logf("cleanup: delete object %s: %v", aws.ToString(obj.Key), err)
				}
			}
		}

		_, err := be.Client().DeleteBucket(ctx, &awss3.DeleteBucketInput{
			Bucket: aws.String(bucket),
		})
		if err != nil {
			t.Logf("cleanup: delete bucket %s: %v", bucket, err)
		}
	})

	return be, bucket
}

// TestE2EFullFlowS3 exercises the complete backup → forget → prune → check →
// restore → verify flow against a MinIO S3-compatible backend.
func TestE2EFullFlowS3(t *testing.T) {
	skipIfNoMinIO(t)
	be, _ := setupTestBucketS3(t)

	// Step 1: Init repository on S3.
	repo, err := repository.Init(be, "testpass")
	if err != nil {
		t.Fatalf("repository.Init: %v", err)
	}

	// Step 2: Create three backups with distinct data and hostnames.
	type backupSpec struct {
		file    string
		content []byte
		host    string
	}
	backups := []backupSpec{
		{"file1.txt", bytes.Repeat([]byte("A"), 4096), "host1"},
		{"file2.txt", bytes.Repeat([]byte("B"), 4096), "host2"},
		{"file3.txt", bytes.Repeat([]byte("C"), 4096), "host3"},
	}

	for i, b := range backups {
		srcDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(srcDir, b.file), b.content, 0644); err != nil {
			t.Fatalf("WriteFile backup %d: %v", i+1, err)
		}
		a := archiver.New(repo)
		if _, _, err := a.Backup([]string{srcDir}, nil, b.host, nil); err != nil {
			t.Fatalf("Backup %d: %v", i+1, err)
		}
		// Small sleep to ensure distinct timestamps for retention ordering.
		time.Sleep(10 * time.Millisecond)
	}

	// Step 3: Re-open repository to reload indexes from S3.
	repo, err = repository.Open(be, "testpass")
	if err != nil {
		t.Fatalf("repository.Open after backups: %v", err)
	}

	// Step 4: Apply retention policy (keep-last 1) to select snapshots to forget.
	snapIDs, err := repo.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snapIDs) != 3 {
		t.Fatalf("snapshot count = %d, want 3", len(snapIDs))
	}

	var snapInfos []retention.SnapshotInfo
	for _, id := range snapIDs {
		snap, err := repo.LoadSnapshot(id)
		if err != nil {
			t.Fatalf("LoadSnapshot(%s): %v", id, err)
		}
		snapInfos = append(snapInfos, retention.SnapshotInfo{
			ID:       id,
			Time:     snap.Time,
			Hostname: snap.Hostname,
			Paths:    snap.Paths,
			Tags:     snap.Tags,
		})
	}

	result, err := retention.Apply(snapInfos, retention.Policy{KeepLast: 1}, "", "")
	if err != nil {
		t.Fatalf("retention.Apply: %v", err)
	}
	if len(result.Keep) != 1 {
		t.Fatalf("retention keep = %d, want 1", len(result.Keep))
	}
	if len(result.Forget) != 2 {
		t.Fatalf("retention forget = %d, want 2", len(result.Forget))
	}

	keptSnapID := result.Keep[0].ID

	// Step 5: Remove forgotten snapshots.
	for _, f := range result.Forget {
		if err := repo.RemoveSnapshot(f.ID); err != nil {
			t.Fatalf("RemoveSnapshot(%s): %v", f.ID, err)
		}
	}

	// Step 6: Prune unreferenced data.
	p := New(repo)
	stats, err := p.Prune(Options{}, nopProgress)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if stats.PacksDeleted+stats.PacksRepacked == 0 {
		t.Error("expected some packs to be deleted or repacked after forgetting 2 of 3 snapshots")
	}
	t.Logf("Prune stats: deleted=%d repacked=%d bytes_freed=%d",
		stats.PacksDeleted, stats.PacksRepacked, stats.BytesFreed)

	// Step 7: Re-open repository after prune and run integrity check.
	repo, err = repository.Open(be, "testpass")
	if err != nil {
		t.Fatalf("repository.Open after prune: %v", err)
	}

	c := checker.New(repo)
	errs, checkErr := c.Check(true, func(string) {})
	if checkErr != nil {
		t.Fatalf("Check after prune: %v", checkErr)
	}
	if len(errs) != 0 {
		for _, e := range errs {
			t.Errorf("check error: %s", e.Message)
		}
		t.Fatalf("expected 0 check errors after prune, got %d", len(errs))
	}

	// Step 8: Verify only one snapshot remains.
	remainingSnaps, err := repo.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots after prune: %v", err)
	}
	if len(remainingSnaps) != 1 {
		t.Fatalf("remaining snapshots = %d, want 1", len(remainingSnaps))
	}
	if remainingSnaps[0] != keptSnapID {
		t.Errorf("remaining snapshot ID = %s, want %s", remainingSnaps[0], keptSnapID)
	}

	// Step 9: Restore the surviving snapshot and verify content.
	restoreDir := t.TempDir()
	r := restorer.New(repo)
	if err := r.Restore(keptSnapID, restoreDir, nil); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// The kept snapshot is the newest (host3, file3.txt with 'C' bytes).
	// Walk the restore dir to find the file.
	var foundFile string
	err = filepath.Walk(restoreDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == "file3.txt" {
			foundFile = path
		}
		return nil
	})
	if err != nil {
		t.Fatalf("filepath.Walk: %v", err)
	}
	if foundFile == "" {
		t.Fatal("restored file3.txt not found in restore directory")
	}

	got, err := os.ReadFile(foundFile)
	if err != nil {
		t.Fatalf("ReadFile restored file: %v", err)
	}
	expected := bytes.Repeat([]byte("C"), 4096)
	if !bytes.Equal(got, expected) {
		t.Errorf("restored content mismatch: got %d bytes, want %d bytes", len(got), len(expected))
	}
}
