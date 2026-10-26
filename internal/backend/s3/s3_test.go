//go:build integration

package s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/hooperits/hitstor/internal/backend"
)

// testID creates a deterministic backend.ID from a string by hashing it.
func testID(s string) backend.ID {
	return sha256.Sum256([]byte(s))
}

// skipIfNoMinIO skips the test if MinIO is not reachable at localhost:9000.
func skipIfNoMinIO(t *testing.T) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", "localhost:9000", 2*time.Second)
	if err != nil {
		t.Skip("MinIO not available at localhost:9000, skipping integration tests")
	}
	conn.Close()
}

// setupTestBucket creates a unique S3 bucket for testing and registers
// cleanup to delete all objects and the bucket when the test finishes.
func setupTestBucket(t *testing.T) (*S3Backend, string) {
	t.Helper()

	bucket := fmt.Sprintf("hitstor-test-%d", time.Now().UnixNano())
	be, err := New("http://localhost:9000", bucket, "minioadmin", "minioadmin", "us-east-1")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Create the bucket.
	_, err = be.Client().CreateBucket(context.Background(), &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		t.Fatalf("CreateBucket(%s): %v", bucket, err)
	}

	t.Cleanup(func() {
		ctx := context.Background()

		// List and delete all objects in the bucket.
		paginator := s3.NewListObjectsV2Paginator(be.Client(), &s3.ListObjectsV2Input{
			Bucket: aws.String(bucket),
		})
		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				t.Logf("cleanup: list objects: %v", err)
				break
			}
			for _, obj := range page.Contents {
				_, err := be.Client().DeleteObject(ctx, &s3.DeleteObjectInput{
					Bucket: aws.String(bucket),
					Key:    obj.Key,
				})
				if err != nil {
					t.Logf("cleanup: delete object %s: %v", aws.ToString(obj.Key), err)
				}
			}
		}

		// Delete the bucket.
		_, err := be.Client().DeleteBucket(ctx, &s3.DeleteBucketInput{
			Bucket: aws.String(bucket),
		})
		if err != nil {
			t.Logf("cleanup: delete bucket %s: %v", bucket, err)
		}
	})

	return be, bucket
}

func TestSaveThenLoad(t *testing.T) {
	skipIfNoMinIO(t)
	be, _ := setupTestBucket(t)

	ctx := context.Background()
	id := testID("test-save-load")
	data := []byte("hello, hitstor S3 backend")

	if err := be.Save(ctx, backend.DataFile, id, bytes.NewReader(data)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	rc, err := be.Load(ctx, backend.DataFile, id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if !bytes.Equal(got, data) {
		t.Errorf("Load returned %q, want %q", got, data)
	}
}

func TestStatReturnsCorrectSize(t *testing.T) {
	skipIfNoMinIO(t)
	be, _ := setupTestBucket(t)

	ctx := context.Background()
	id := testID("test-stat")
	data := []byte("some test data for stat on S3")

	if err := be.Save(ctx, backend.DataFile, id, bytes.NewReader(data)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	fi, err := be.Stat(ctx, backend.DataFile, id)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	if fi.Size != int64(len(data)) {
		t.Errorf("Stat size = %d, want %d", fi.Size, len(data))
	}
}

func TestListReturnsSavedIDs(t *testing.T) {
	skipIfNoMinIO(t)
	be, _ := setupTestBucket(t)

	ctx := context.Background()
	id1 := testID("list-item-1")
	id2 := testID("list-item-2")
	id3 := testID("list-item-3")

	for _, id := range []backend.ID{id1, id2, id3} {
		if err := be.Save(ctx, backend.IndexFile, id, bytes.NewReader([]byte("data"))); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	ids, err := be.List(ctx, backend.IndexFile)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(ids) != 3 {
		t.Fatalf("List returned %d IDs, want 3", len(ids))
	}

	expected := map[backend.ID]bool{id1: true, id2: true, id3: true}
	for _, id := range ids {
		if !expected[id] {
			t.Errorf("List returned unexpected ID %s", id)
		}
		delete(expected, id)
	}
	for id := range expected {
		t.Errorf("List missing expected ID %s", id)
	}
}

func TestRemoveDeletesFile(t *testing.T) {
	skipIfNoMinIO(t)
	be, _ := setupTestBucket(t)

	ctx := context.Background()
	id := testID("test-remove")
	data := []byte("data to remove from S3")

	if err := be.Save(ctx, backend.DataFile, id, bytes.NewReader(data)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify it exists first.
	rc, err := be.Load(ctx, backend.DataFile, id)
	if err != nil {
		t.Fatalf("Load before Remove: %v", err)
	}
	rc.Close()

	if err := be.Remove(ctx, backend.DataFile, id); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Load should now fail.
	_, err = be.Load(ctx, backend.DataFile, id)
	if err == nil {
		t.Error("Load after Remove should have returned error, got nil")
	}
}

func TestDataFileUsesCorrectKeyPrefix(t *testing.T) {
	skipIfNoMinIO(t)
	be, bucket := setupTestBucket(t)

	ctx := context.Background()
	id := testID("test-key-prefix")
	idHex := id.String()
	data := []byte("key prefix test data")

	if err := be.Save(ctx, backend.DataFile, id, bytes.NewReader(data)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify the object is stored at data/{hex[:2]}/{hex}.
	expectedKey := fmt.Sprintf("data/%s/%s", idHex[:2], idHex)

	// List objects in the bucket and find our key.
	output, err := be.Client().ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String("data/"),
	})
	if err != nil {
		t.Fatalf("ListObjectsV2: %v", err)
	}

	found := false
	for _, obj := range output.Contents {
		key := aws.ToString(obj.Key)
		if key == expectedKey {
			found = true
			break
		}
	}

	if !found {
		var keys []string
		for _, obj := range output.Contents {
			keys = append(keys, aws.ToString(obj.Key))
		}
		t.Errorf("expected key %s not found; got keys: %v", expectedKey, keys)
	}

	// Also verify the key starts with the correct fan-out prefix.
	if !strings.HasPrefix(expectedKey, "data/"+idHex[:2]+"/") {
		t.Errorf("key %s does not have expected prefix data/%s/", expectedKey, idHex[:2])
	}
}

func TestSaveToAllFileTypes(t *testing.T) {
	skipIfNoMinIO(t)
	be, _ := setupTestBucket(t)

	ctx := context.Background()

	fileTypes := []struct {
		ft   backend.FileType
		name string
	}{
		{backend.DataFile, "data"},
		{backend.IndexFile, "index"},
		{backend.SnapshotFile, "snapshot"},
		{backend.KeyFile, "key"},
		{backend.LockFile, "lock"},
		{backend.ConfigFile, "config"},
	}

	for _, tc := range fileTypes {
		t.Run(tc.name, func(t *testing.T) {
			id := testID("filetype-" + tc.name)
			data := []byte("data for " + tc.name)

			if err := be.Save(ctx, tc.ft, id, bytes.NewReader(data)); err != nil {
				t.Fatalf("Save(%s): %v", tc.name, err)
			}

			rc, err := be.Load(ctx, tc.ft, id)
			if err != nil {
				t.Fatalf("Load(%s): %v", tc.name, err)
			}
			defer rc.Close()

			got, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("ReadAll(%s): %v", tc.name, err)
			}

			if !bytes.Equal(got, data) {
				t.Errorf("Load(%s) = %q, want %q", tc.name, got, data)
			}
		})
	}
}
