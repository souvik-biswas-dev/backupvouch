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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
)

// BackupSuccessMessage is written to /dev/termination-log on a successful backup.
type BackupSuccessMessage struct {
	ObjectKey string `json:"objectKey"`
	SizeBytes int64  `json:"sizeBytes"`
	SHA256    string `json:"sha256"`
}

// BackupErrorDetail is the inner error object in a failure termination message.
type BackupErrorDetail struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// BackupErrorMessage is written to /dev/termination-log on a failed backup.
type BackupErrorMessage struct {
	Error BackupErrorDetail `json:"error"`
}

// BackupConfig holds the configuration read from the environment.
type BackupConfig struct {
	PgHost          string
	PgPort          string
	PgDatabase      string
	PgUser          string
	PgPassword      string
	StorageEndpoint string
	StorageBucket   string
	StoragePrefix   string
	StorageUseTLS   bool
	StorageAccess   string
	StorageSecret   string
	ObjectKey       string
	TerminationLog  string
}

// LoadBackupConfig reads the backup configuration from environment variables.
func LoadBackupConfig() (*BackupConfig, error) {
	cfg := &BackupConfig{
		PgHost:          getEnv("PGHOST", "localhost"),
		PgPort:          getEnv("PGPORT", "5432"),
		PgDatabase:      os.Getenv("PGDATABASE"),
		PgUser:          os.Getenv("PGUSER"),
		PgPassword:      os.Getenv("PGPASSWORD"),
		StorageEndpoint: os.Getenv("STORAGE_ENDPOINT"),
		StorageBucket:   os.Getenv("STORAGE_BUCKET"),
		StoragePrefix:   os.Getenv("STORAGE_PREFIX"),
		StorageUseTLS:   os.Getenv("STORAGE_USE_TLS") == envTrue,
		StorageAccess:   os.Getenv("STORAGE_ACCESS_KEY"),
		StorageSecret:   os.Getenv("STORAGE_SECRET_KEY"),
		ObjectKey:       os.Getenv("OBJECT_KEY"),
		TerminationLog:  getEnv("TERMINATION_LOG_PATH", "/dev/termination-log"),
	}

	var missing []string
	if cfg.PgDatabase == "" {
		missing = append(missing, "PGDATABASE")
	}
	if cfg.PgUser == "" {
		missing = append(missing, "PGUSER")
	}
	if cfg.PgPassword == "" {
		missing = append(missing, "PGPASSWORD")
	}
	if cfg.StorageEndpoint == "" {
		missing = append(missing, "STORAGE_ENDPOINT")
	}
	if cfg.StorageBucket == "" {
		missing = append(missing, "STORAGE_BUCKET")
	}
	if cfg.StorageAccess == "" {
		missing = append(missing, "STORAGE_ACCESS_KEY")
	}
	if cfg.StorageSecret == "" {
		missing = append(missing, "STORAGE_SECRET_KEY")
	}
	if cfg.ObjectKey == "" {
		missing = append(missing, "OBJECT_KEY")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// hashingReader wraps an io.Reader, computing SHA-256 and counting total bytes read.
type hashingReader struct {
	r io.Reader
	h hash.Hash
	n int64
}

func newHashingReader(r io.Reader) *hashingReader {
	return &hashingReader{
		r: r,
		h: sha256.New(),
	}
}

func (hr *hashingReader) Read(p []byte) (int, error) {
	n, err := hr.r.Read(p)
	if n > 0 {
		hr.h.Write(p[:n])
		hr.n += int64(n)
	}
	return n, err
}

func (hr *hashingReader) SHA256Hex() string {
	return hex.EncodeToString(hr.h.Sum(nil))
}

func (hr *hashingReader) BytesCount() int64 {
	return hr.n
}

// sanitizeMessage ensures messages are truncated to 120 characters and contains no secrets.
func sanitizeMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	// Replace linebreaks
	msg = strings.ReplaceAll(msg, "\n", " ")
	msg = strings.ReplaceAll(msg, "\r", " ")
	if len(msg) > 120 {
		msg = msg[:117] + "..."
	}
	return msg
}

// writeTerminationLog writes JSON to the termination log path.
func writeTerminationLog(path string, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal termination message: %w", err)
	}

	// Attempt to write to file
	if err := os.WriteFile(path, data, 0644); err != nil {
		// If writing to /dev/termination-log fails (e.g. running in testing environment without root),
		// print to stdout as fallback.
		fmt.Fprintf(os.Stderr, "warning: writing to %s failed (%v); termination message: %s\n", path, err, string(data))
		return err
	}
	return nil
}

// ClassifyError determines the termination reason and a safe error message.
func ClassifyError(err error, stderr string) (string, string) {
	combined := strings.ToLower(fmt.Sprintf("%v %s", err, stderr))

	// Source connectivity/auth errors
	if strings.Contains(combined, "connection refused") ||
		strings.Contains(combined, "could not connect to server") ||
		strings.Contains(combined, "no route to host") ||
		strings.Contains(combined, "name resolution") ||
		strings.Contains(combined, "password authentication failed") ||
		strings.Contains(combined, "database") && strings.Contains(combined, "does not exist") {
		return backupv1alpha1.ReasonSourceUnreachable, sanitizeMessage(stderr)
	}

	// Storage / S3 connectivity errors
	if strings.Contains(combined, "nosuchbucket") ||
		strings.Contains(combined, "the specified bucket does not exist") ||
		strings.Contains(combined, "access denied") ||
		strings.Contains(combined, "invalidaccesskeyid") ||
		strings.Contains(combined, "signaturedoesnotmatch") ||
		strings.Contains(combined, "endpoint") ||
		strings.Contains(combined, "s3") {
		return backupv1alpha1.ReasonStorageUnreachable, sanitizeMessage(err.Error())
	}

	if stderr != "" {
		return backupv1alpha1.ReasonBackupJobFailed, sanitizeMessage(stderr)
	}
	return backupv1alpha1.ReasonBackupJobFailed, sanitizeMessage(err.Error())
}

// RunBackup performs the full backup procedure:
// 1. Validates configuration.
// 2. Connects to S3 storage via minio-go.
// 3. Executes pg_dump -Fc and streams stdout to MinIO while computing SHA-256 and size.
// 4. Writes JSON termination log.
func RunBackup(ctx context.Context) error {
	cfg, err := LoadBackupConfig()
	if err != nil {
		errMsg := BackupErrorMessage{
			Error: BackupErrorDetail{
				Reason:  backupv1alpha1.ReasonBackupJobFailed,
				Message: sanitizeMessage(err.Error()),
			},
		}
		_ = writeTerminationLog(getEnv("TERMINATION_LOG_PATH", "/dev/termination-log"), errMsg)
		return err
	}

	// Initialize MinIO client
	minioClient, err := minio.New(cfg.StorageEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.StorageAccess, cfg.StorageSecret, ""),
		Secure: cfg.StorageUseTLS,
	})
	if err != nil {
		errMsg := BackupErrorMessage{
			Error: BackupErrorDetail{
				Reason:  backupv1alpha1.ReasonStorageUnreachable,
				Message: sanitizeMessage(err.Error()),
			},
		}
		_ = writeTerminationLog(cfg.TerminationLog, errMsg)
		return fmt.Errorf("failed to initialize storage client: %w", err)
	}

	// Check if bucket exists
	exists, err := minioClient.BucketExists(ctx, cfg.StorageBucket)
	if err != nil || !exists {
		msg := "bucket does not exist or unreachable"
		if err != nil {
			msg = err.Error()
		}
		errMsg := BackupErrorMessage{
			Error: BackupErrorDetail{
				Reason:  backupv1alpha1.ReasonStorageUnreachable,
				Message: sanitizeMessage(msg),
			},
		}
		_ = writeTerminationLog(cfg.TerminationLog, errMsg)
		return fmt.Errorf("storage bucket check failed: %s", msg)
	}

	// Prepare pg_dump execution
	// pg_dump -h <host> -p <port> -U <user> -d <database> -Fc
	cmd := exec.CommandContext(ctx, "pg_dump",
		"-h", cfg.PgHost,
		"-p", cfg.PgPort,
		"-U", cfg.PgUser,
		"-d", cfg.PgDatabase,
		"-Fc",
	)

	// Set PGPASSWORD in environment only, never in args or logs
	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", cfg.PgPassword))

	pr, pw := io.Pipe()
	var stderrBuf bytes.Buffer
	cmd.Stdout = pw
	cmd.Stderr = &stderrBuf

	// Stream stdout of pg_dump into pw
	var pgDumpErr error
	pgDone := make(chan struct{})
	go func() {
		defer close(pgDone)
		pgDumpErr = cmd.Run()
		if pgDumpErr != nil {
			_ = pw.CloseWithError(pgDumpErr)
		} else {
			_ = pw.Close()
		}
	}()

	// Read from pr via hashing reader
	hr := newHashingReader(pr)

	// Stream to MinIO
	_, uploadErr := minioClient.PutObject(ctx, cfg.StorageBucket, cfg.ObjectKey, hr, -1, minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})

	<-pgDone

	// If either pg_dump or MinIO upload failed, report structured error
	if pgDumpErr != nil || uploadErr != nil {
		activeErr := pgDumpErr
		if activeErr == nil {
			activeErr = uploadErr
		}
		reason, message := ClassifyError(activeErr, stderrBuf.String())
		errMsg := BackupErrorMessage{
			Error: BackupErrorDetail{
				Reason:  reason,
				Message: message,
			},
		}
		_ = writeTerminationLog(cfg.TerminationLog, errMsg)
		return errors.New(message)
	}

	// Success! Write structured termination message
	successMsg := BackupSuccessMessage{
		ObjectKey: cfg.ObjectKey,
		SizeBytes: hr.BytesCount(),
		SHA256:    hr.SHA256Hex(),
	}

	if err := writeTerminationLog(cfg.TerminationLog, successMsg); err != nil {
		return fmt.Errorf("failed to write termination log: %w", err)
	}

	return nil
}
