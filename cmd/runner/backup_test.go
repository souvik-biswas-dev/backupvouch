/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
)

func TestSanitizeMessage(t *testing.T) {
	shortMsg := "could not connect to server: Connection refused"
	if got := sanitizeMessage(shortMsg); got != shortMsg {
		t.Errorf("sanitizeMessage(%q) = %q, expected unchanged", shortMsg, got)
	}

	longMsg := strings.Repeat("a", 150)
	got := sanitizeMessage(longMsg)
	if len(got) > 120 {
		t.Errorf("expected len <= 120, got %d", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("expected '...' suffix on truncated message")
	}

	multilineMsg := "line 1\nline 2\r\nline 3"
	if got := sanitizeMessage(multilineMsg); strings.Contains(got, "\n") || strings.Contains(got, "\r") {
		t.Errorf("expected newlines to be stripped: %q", got)
	}
}

func TestHashingReader(t *testing.T) {
	data := []byte("hello BackupVouch postgres dump stream")
	r := strings.NewReader(string(data))
	hr := newHashingReader(r)

	buf, err := io.ReadAll(hr)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	if string(buf) != string(data) {
		t.Errorf("read mismatch: %q vs %q", string(buf), string(data))
	}

	expectedHash := sha256.Sum256(data)
	expectedHex := hex.EncodeToString(expectedHash[:])

	if hr.SHA256Hex() != expectedHex {
		t.Errorf("SHA256Hex = %q, expected %q", hr.SHA256Hex(), expectedHex)
	}
	if hr.BytesCount() != int64(len(data)) {
		t.Errorf("BytesCount = %d, expected %d", hr.BytesCount(), len(data))
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		stderr         string
		expectedReason string
	}{
		{
			name:           "connection refused is SourceUnreachable",
			err:            errors.New("exit status 1"),
			stderr:         "pg_dump: error: connection to server at \"10.0.0.1\", port 5432 failed: Connection refused",
			expectedReason: backupv1alpha1.ReasonSourceUnreachable,
		},
		{
			name:           "password authentication failed is SourceUnreachable",
			err:            errors.New("exit status 1"),
			stderr:         "pg_dump: error: password authentication failed for user \"app\"",
			expectedReason: backupv1alpha1.ReasonSourceUnreachable,
		},
		{
			name:           "database does not exist is SourceUnreachable",
			err:            errors.New("exit status 1"),
			stderr:         "pg_dump: error: database \"missing\" does not exist",
			expectedReason: backupv1alpha1.ReasonSourceUnreachable,
		},
		{
			name:           "NoSuchBucket is StorageUnreachable",
			err:            errors.New("The specified bucket does not exist (NoSuchBucket)"),
			stderr:         "",
			expectedReason: backupv1alpha1.ReasonStorageUnreachable,
		},
		{
			name:           "access denied S3 is StorageUnreachable",
			err:            errors.New("Access Denied to S3 endpoint"),
			stderr:         "",
			expectedReason: backupv1alpha1.ReasonStorageUnreachable,
		},
		{
			name:           "generic failure is BackupJobFailed",
			err:            errors.New("unexpected pipe error"),
			stderr:         "internal write error",
			expectedReason: backupv1alpha1.ReasonBackupJobFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, _ := ClassifyError(tt.err, tt.stderr)
			if reason != tt.expectedReason {
				t.Errorf("ClassifyError reason = %q, expected %q", reason, tt.expectedReason)
			}
		})
	}
}

func TestWriteTerminationLog(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "termination-log")

	success := BackupSuccessMessage{
		ObjectKey: "app/2026-10-24T02-00-00Z.dump",
		SizeBytes: 4096,
		SHA256:    "abcd1234ef567890",
	}

	if err := writeTerminationLog(logPath, success); err != nil {
		t.Fatalf("failed to write termination log: %v", err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read termination log: %v", err)
	}

	var readMsg BackupSuccessMessage
	if err := json.Unmarshal(content, &readMsg); err != nil {
		t.Fatalf("failed to unmarshal written json: %v", err)
	}

	if readMsg != success {
		t.Errorf("unmarshaled msg %+v != original %+v", readMsg, success)
	}

	// Verify size is well within 3500 bytes budget
	if len(content) > 3500 {
		t.Errorf("termination message length %d exceeds 3500 bytes", len(content))
	}
}

func TestLoadBackupConfig(t *testing.T) {
	t.Run("missing required env returns error", func(t *testing.T) {
		os.Clearenv()
		_, err := LoadBackupConfig()
		if err == nil {
			t.Errorf("expected error when required env vars are missing")
		}
	})

	t.Run("all required env provided succeeds", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("PGDATABASE", "app")
		os.Setenv("PGUSER", "appuser")
		os.Setenv("PGPASSWORD", "secret")
		os.Setenv("STORAGE_ENDPOINT", "minio:9000")
		os.Setenv("STORAGE_BUCKET", "backups")
		os.Setenv("STORAGE_ACCESS_KEY", "access")
		os.Setenv("STORAGE_SECRET_KEY", "secret")
		os.Setenv("OBJECT_KEY", "app/dump.dump")

		cfg, err := LoadBackupConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.PgDatabase != "app" || cfg.StorageEndpoint != "minio:9000" {
			t.Errorf("unexpected config loaded: %+v", cfg)
		}
		if cfg.PgHost != "localhost" || cfg.PgPort != "5432" {
			t.Errorf("default host/port not applied: %+v", cfg)
		}
	})
}
