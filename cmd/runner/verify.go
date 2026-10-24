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
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
	"github.com/souvik-biswas-dev/backupvouch/internal/checks"
)

// VerifyResultMessage is written to /dev/termination-log on completion of verification.
type VerifyResultMessage struct {
	Result                 backupv1alpha1.VerificationResultType `json:"result"`
	Reason                 string                                `json:"reason,omitempty"`
	RestoreDurationSeconds float64                               `json:"restoreDurationSeconds"`
	Checks                 []checks.CheckResult                  `json:"checks,omitempty"`
	ChecksTruncated        bool                                  `json:"checksTruncated,omitempty"`
}

// VerifyConfig holds parameters for the verify subcommand read from environment variables.
type VerifyConfig struct {
	StorageEndpoint string
	StorageBucket   string
	StorageUseTLS   bool
	StorageAccess   string
	StorageSecret   string
	ObjectKey       string
	ExpectedSHA256  string
	DatabaseName    string
	ChecksJSON      string
	ScratchDir      string
	TerminationLog  string
}

// LoadVerifyConfig loads verification configuration from the environment.
func LoadVerifyConfig() (*VerifyConfig, error) {
	cfg := &VerifyConfig{
		StorageEndpoint: os.Getenv("STORAGE_ENDPOINT"),
		StorageBucket:   os.Getenv("STORAGE_BUCKET"),
		StorageUseTLS:   os.Getenv("STORAGE_USE_TLS") == envTrue,
		StorageAccess:   os.Getenv("STORAGE_ACCESS_KEY"),
		StorageSecret:   os.Getenv("STORAGE_SECRET_KEY"),
		ObjectKey:       os.Getenv("OBJECT_KEY"),
		ExpectedSHA256:  os.Getenv("EXPECTED_SHA256"),
		DatabaseName:    getEnv("DATABASE_NAME", "app"),
		ChecksJSON:      os.Getenv("CHECKS_JSON"),
		ScratchDir:      getEnv("SCRATCH_DIR", "/scratch"),
		TerminationLog:  getEnv("TERMINATION_LOG_PATH", "/dev/termination-log"),
	}

	var missing []string
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
	if cfg.ExpectedSHA256 == "" {
		missing = append(missing, "EXPECTED_SHA256")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}

// BuildVerifyResult constructs the VerifyResultMessage, truncating passed checks if the JSON exceeds 3500 bytes.
func BuildVerifyResult(
	result backupv1alpha1.VerificationResultType,
	reason string,
	durationSeconds float64,
	checkResults []checks.CheckResult,
) VerifyResultMessage {
	msg := VerifyResultMessage{
		Result:                 result,
		Reason:                 reason,
		RestoreDurationSeconds: durationSeconds,
		Checks:                 checkResults,
	}

	data, err := json.Marshal(msg)
	if err == nil && len(data) <= 3500 {
		return msg
	}

	// Truncate: keep failed checks, drop passed checks, set checksTruncated: true
	failedOnly := make([]checks.CheckResult, 0)
	for _, c := range checkResults {
		if !c.Passed {
			failedOnly = append(failedOnly, c)
		}
	}

	return VerifyResultMessage{
		Result:                 result,
		Reason:                 reason,
		RestoreDurationSeconds: durationSeconds,
		Checks:                 failedOnly,
		ChecksTruncated:        true,
	}
}

// downloadAndHash downloads the object from MinIO and saves it to dstFile while computing its SHA-256.
func downloadAndHash(ctx context.Context, client *minio.Client, bucket, objectKey, dstFile string) (string, error) {
	obj, err := client.GetObject(ctx, bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return "", err
	}
	defer func() {
		_ = obj.Close()
	}()

	out, err := os.Create(dstFile)
	if err != nil {
		return "", fmt.Errorf("failed to create destination dump file: %w", err)
	}
	defer func() {
		_ = out.Close()
	}()

	hasher := sha256.New()
	multiWriter := io.MultiWriter(out, hasher)

	if _, err := io.Copy(multiWriter, obj); err != nil {
		return "", fmt.Errorf("failed while streaming object download: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// RunVerify executes the full verification lifecycle:
// 1. Download backup object and verify SHA-256.
// 2. Start scratch Postgres on Unix domain socket in scratchDir.
// 3. Run pg_restore into scratch Postgres.
// 4. Run SQL checks and collect pass/fail results.
// 5. Always shut down scratch Postgres.
// 6. Write JSON termination message.
func RunVerify(ctx context.Context) error {
	cfg, err := LoadVerifyConfig()
	if err != nil {
		res := BuildVerifyResult(backupv1alpha1.VerificationFailed, backupv1alpha1.ReasonInvalidRunnerResult, 0, nil)
		_ = writeTerminationLog(getEnv("TERMINATION_LOG_PATH", "/dev/termination-log"), res)
		return err
	}

	// 1. Initialize MinIO client
	minioClient, err := minio.New(cfg.StorageEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.StorageAccess, cfg.StorageSecret, ""),
		Secure: cfg.StorageUseTLS,
	})
	if err != nil {
		res := BuildVerifyResult(backupv1alpha1.VerificationFailed, backupv1alpha1.ReasonStorageUnreachable, 0, nil)
		_ = writeTerminationLog(cfg.TerminationLog, res)
		return fmt.Errorf("failed to initialize storage client: %w", err)
	}

	// 2. Download and verify checksum
	dumpFile := filepath.Join(cfg.ScratchDir, "backup.dump")
	actualSHA256, err := downloadAndHash(ctx, minioClient, cfg.StorageBucket, cfg.ObjectKey, dumpFile)
	if err != nil {
		res := BuildVerifyResult(backupv1alpha1.VerificationFailed, backupv1alpha1.ReasonStorageUnreachable, 0, nil)
		_ = writeTerminationLog(cfg.TerminationLog, res)
		return fmt.Errorf("failed to download backup: %w", err)
	}

	if !strings.EqualFold(actualSHA256, cfg.ExpectedSHA256) {
		res := BuildVerifyResult(backupv1alpha1.VerificationFailed, backupv1alpha1.ReasonChecksumMismatch, 0, nil)
		_ = writeTerminationLog(cfg.TerminationLog, res)
		return fmt.Errorf("checksum mismatch: expected %s, got %s", cfg.ExpectedSHA256, actualSHA256)
	}

	// 3. Start throwaway scratch Postgres
	dataDir := filepath.Join(cfg.ScratchDir, "data")
	socketDir := filepath.Join(cfg.ScratchDir, "socket")
	_ = os.RemoveAll(dataDir)
	_ = os.RemoveAll(socketDir)
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		res := BuildVerifyResult(backupv1alpha1.VerificationFailed, backupv1alpha1.ReasonRestoreFailed, 0, nil)
		_ = writeTerminationLog(cfg.TerminationLog, res)
		return fmt.Errorf("failed to create data dir: %w", err)
	}
	if err := os.MkdirAll(socketDir, 0755); err != nil {
		res := BuildVerifyResult(backupv1alpha1.VerificationFailed, backupv1alpha1.ReasonRestoreFailed, 0, nil)
		_ = writeTerminationLog(cfg.TerminationLog, res)
		return fmt.Errorf("failed to create socket dir: %w", err)
	}

	// Clean up Postgres server and scratch files when finished
	defer func() {
		_ = exec.Command("pg_ctl", "-D", dataDir, "-m", "immediate", "stop").Run()
		_ = os.RemoveAll(dataDir)
		_ = os.RemoveAll(socketDir)
		_ = os.Remove(dumpFile)
	}()

	initCmd := exec.CommandContext(ctx, "initdb", "-D", dataDir, "-U", "postgres", "-A", "trust", "--no-sync")
	if out, err := initCmd.CombinedOutput(); err != nil {
		res := BuildVerifyResult(backupv1alpha1.VerificationFailed, backupv1alpha1.ReasonRestoreFailed, 0, nil)
		_ = writeTerminationLog(cfg.TerminationLog, res)
		return fmt.Errorf("initdb failed (%w): %s", err, string(out))
	}

	// Start Postgres listening ONLY on the Unix socket, no network interface (-h '')
	pgCtlOpts := fmt.Sprintf("-k %s -h ''", socketDir)
	startCmd := exec.CommandContext(ctx, "pg_ctl", "-D", dataDir, "-o", pgCtlOpts, "-w", "start")
	if out, err := startCmd.CombinedOutput(); err != nil {
		res := BuildVerifyResult(backupv1alpha1.VerificationFailed, backupv1alpha1.ReasonRestoreFailed, 0, nil)
		_ = writeTerminationLog(cfg.TerminationLog, res)
		return fmt.Errorf("pg_ctl start failed (%w): %s", err, string(out))
	}

	// 4. Create target database and restore
	_ = exec.CommandContext(ctx, "createdb", "-h", socketDir, "-U", "postgres", cfg.DatabaseName).Run()

	startRestore := time.Now()
	restoreCmd := exec.CommandContext(ctx, "pg_restore",
		"-h", socketDir,
		"-U", "postgres",
		"-d", cfg.DatabaseName,
		"--no-owner",
		"--no-privileges",
		dumpFile,
	)
	if out, err := restoreCmd.CombinedOutput(); err != nil {
		duration := time.Since(startRestore).Seconds()
		res := BuildVerifyResult(backupv1alpha1.VerificationFailed, backupv1alpha1.ReasonRestoreFailed, duration, nil)
		_ = writeTerminationLog(cfg.TerminationLog, res)
		return fmt.Errorf("pg_restore failed (%w): %s", err, string(out))
	}
	restoreDuration := time.Since(startRestore).Seconds()

	// 5. Run SQL checks if specified
	var parsedChecks []backupv1alpha1.Check
	if cfg.ChecksJSON != "" {
		if err := json.Unmarshal([]byte(cfg.ChecksJSON), &parsedChecks); err != nil {
			res := BuildVerifyResult(
				backupv1alpha1.VerificationFailed,
				backupv1alpha1.ReasonInvalidRunnerResult,
				restoreDuration,
				nil,
			)
			_ = writeTerminationLog(cfg.TerminationLog, res)
			return fmt.Errorf("failed to parse checks JSON: %w", err)
		}
	}

	connStr := fmt.Sprintf("host=%s user=postgres dbname=%s sslmode=disable", socketDir, cfg.DatabaseName)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		res := BuildVerifyResult(backupv1alpha1.VerificationFailed, backupv1alpha1.ReasonRestoreFailed, restoreDuration, nil)
		_ = writeTerminationLog(cfg.TerminationLog, res)
		return fmt.Errorf("failed to connect to restored database: %w", err)
	}
	defer func() {
		_ = db.Close()
	}()

	checkResults, allPassed, err := checks.RunChecks(ctx, db, parsedChecks)
	if err != nil {
		res := BuildVerifyResult(
			backupv1alpha1.VerificationFailed,
			backupv1alpha1.ReasonCheckFailed,
			restoreDuration,
			checkResults,
		)
		_ = writeTerminationLog(cfg.TerminationLog, res)
		return err
	}

	if !allPassed {
		res := BuildVerifyResult(
			backupv1alpha1.VerificationFailed,
			backupv1alpha1.ReasonCheckFailed,
			restoreDuration,
			checkResults,
		)
		_ = writeTerminationLog(cfg.TerminationLog, res)
		return errors.New("one or more checks failed")
	}

	// 6. Success!
	res := BuildVerifyResult(backupv1alpha1.VerificationPassed, "", restoreDuration, checkResults)
	if err := writeTerminationLog(cfg.TerminationLog, res); err != nil {
		return fmt.Errorf("failed to write termination log: %w", err)
	}

	return nil
}
