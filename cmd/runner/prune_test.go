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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPruneConfig(t *testing.T) {
	t.Run("missing required variables returns error", func(t *testing.T) {
		t.Setenv("STORAGE_ENDPOINT", "")
		t.Setenv("STORAGE_BUCKET", "")
		t.Setenv("STORAGE_ACCESS_KEY", "")
		t.Setenv("STORAGE_SECRET_KEY", "")

		_, err := LoadPruneConfig()
		if err == nil {
			t.Fatal("expected error for missing environment variables, got nil")
		}
	})

	t.Run("valid config loads properly", func(t *testing.T) {
		t.Setenv("STORAGE_ENDPOINT", "minio:9000")
		t.Setenv("STORAGE_BUCKET", "backups")
		t.Setenv("STORAGE_PREFIX", "app/")
		t.Setenv("STORAGE_USE_TLS", "false")
		t.Setenv("STORAGE_ACCESS_KEY", "minioadmin")
		t.Setenv("STORAGE_SECRET_KEY", "minioadmin")
		t.Setenv("KEEP_LAST", "5")
		t.Setenv("PROTECTED_KEY", "app/protected.dump")
		t.Setenv("TERMINATION_LOG_PATH", "/tmp/term-log")

		cfg, err := LoadPruneConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.KeepLast != 5 {
			t.Errorf("KeepLast = %d, expected 5", cfg.KeepLast)
		}
		if cfg.ProtectedKey != "app/protected.dump" {
			t.Errorf("ProtectedKey = %q, expected 'app/protected.dump'", cfg.ProtectedKey)
		}
		if cfg.StorageBucket != "backups" {
			t.Errorf("StorageBucket = %q, expected 'backups'", cfg.StorageBucket)
		}
	})
}

func TestWritePruneTerminationLog(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "term-log")

	msg := PruneResultMessage{
		Deleted: 3,
		Kept:    7,
	}

	if err := writeTerminationLog(logPath, msg); err != nil {
		t.Fatalf("writeTerminationLog failed: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read termination log: %v", err)
	}

	var parsed PruneResultMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal log: %v", err)
	}

	if parsed.Deleted != 3 || parsed.Kept != 7 {
		t.Errorf("parsed = %+v, expected Deleted:3 Kept:7", parsed)
	}
}
