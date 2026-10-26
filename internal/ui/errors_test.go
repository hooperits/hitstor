package ui

import (
	"fmt"
	"testing"
)

func TestFormatError_MissingRepo(t *testing.T) {
	err := fmt.Errorf("open repository: repository: load config: open /tmp/myrepo/config: no such file or directory")
	got := FormatError(err)

	if got.Message != "Repository not found at /tmp/myrepo" {
		t.Errorf("Message = %q, want %q", got.Message, "Repository not found at /tmp/myrepo")
	}
	if got.Hint == "" {
		t.Error("expected non-empty hint for missing repo")
	}
}

func TestFormatError_PasswordRequired(t *testing.T) {
	err := fmt.Errorf("password required: use --password or --password-file")
	got := FormatError(err)

	if got.Message != "Password required" {
		t.Errorf("Message = %q, want %q", got.Message, "Password required")
	}
	if got.Hint != "Use --password, --password-file, or set HITSTOR_PASSWORD" {
		t.Errorf("Hint = %q, want password hint", got.Hint)
	}
}

func TestFormatError_WrongPassword(t *testing.T) {
	err := fmt.Errorf("open repository: repository: wrong password or corrupted key file")
	got := FormatError(err)

	if got.Message != "Incorrect password" {
		t.Errorf("Message = %q, want %q", got.Message, "Incorrect password")
	}
	if got.Hint != "Check your password or password file" {
		t.Errorf("Hint = %q, want password check hint", got.Hint)
	}
}

func TestFormatError_SnapshotNotFound(t *testing.T) {
	err := fmt.Errorf("snapshot \"abc12345\" not found")
	got := FormatError(err)

	if got.Message != "Snapshot not found" {
		t.Errorf("Message = %q, want %q", got.Message, "Snapshot not found")
	}
	if got.Hint != "Run 'hitstor snapshots' to list available snapshots" {
		t.Errorf("Hint = %q, want snapshots list hint", got.Hint)
	}
}

func TestFormatError_MissingRepoFlag(t *testing.T) {
	err := fmt.Errorf("--repo flag is required")
	got := FormatError(err)

	if got.Message != "Repository location not specified" {
		t.Errorf("Message = %q, want %q", got.Message, "Repository location not specified")
	}
	if got.Hint != "Use --repo <path>, set HITSTOR_REPO, or run 'hitstor config set repo <path>'" {
		t.Errorf("Hint = %q, want repo usage hint", got.Hint)
	}
}

func TestFormatError_DefaultFallback(t *testing.T) {
	err := fmt.Errorf("some unknown error occurred")
	got := FormatError(err)

	if got.Message != "some unknown error occurred" {
		t.Errorf("Message = %q, want original error message", got.Message)
	}
	if got.Hint != "" {
		t.Errorf("Hint = %q, want empty hint for unknown error", got.Hint)
	}
}
