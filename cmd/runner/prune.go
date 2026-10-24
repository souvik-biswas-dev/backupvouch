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
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// PruneResultMessage is written to /dev/termination-log on completion of pruning.
type PruneResultMessage struct {
	Deleted int `json:"deleted"`
	Kept    int `json:"kept"`
}

// PruneConfig holds the configuration read from the environment for the prune subcommand.
type PruneConfig struct {
	StorageEndpoint string
	StorageBucket   string
	StoragePrefix   string
	StorageUseTLS   bool
	StorageAccess   string
	StorageSecret   string
	KeepLast        int
	ProtectedKey    string
	TerminationLog  string
}

// LoadPruneConfig reads the prune configuration from environment variables.
func LoadPruneConfig() (*PruneConfig, error) {
	keepLastStr := getEnv("KEEP_LAST", "1")
	keepLast, err := strconv.Atoi(keepLastStr)
	if err != nil || keepLast < 1 {
		keepLast = 1
	}

	cfg := &PruneConfig{
		StorageEndpoint: os.Getenv("STORAGE_ENDPOINT"),
		StorageBucket:   os.Getenv("STORAGE_BUCKET"),
		StoragePrefix:   os.Getenv("STORAGE_PREFIX"),
		StorageUseTLS:   os.Getenv("STORAGE_USE_TLS") == envTrue,
		StorageAccess:   os.Getenv("STORAGE_ACCESS_KEY"),
		StorageSecret:   os.Getenv("STORAGE_SECRET_KEY"),
		KeepLast:        keepLast,
		ProtectedKey:    os.Getenv("PROTECTED_KEY"),
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

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}

// RunPrune lists objects under prefix, sorts by LastModified desc, keeps the newest keepLast,
// skips deleting protectedKey (the newest verified backup), deletes older objects,
// and writes {"deleted":N,"kept":M} to /dev/termination-log.
func RunPrune(ctx context.Context) error {
	cfg, err := LoadPruneConfig()
	if err != nil {
		return err
	}

	minioClient, err := minio.New(cfg.StorageEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.StorageAccess, cfg.StorageSecret, ""),
		Secure: cfg.StorageUseTLS,
	})
	if err != nil {
		return fmt.Errorf("failed to create storage client: %w", err)
	}

	opts := minio.ListObjectsOptions{
		Prefix:    cfg.StoragePrefix,
		Recursive: true,
	}

	var objects []minio.ObjectInfo
	for obj := range minioClient.ListObjects(ctx, cfg.StorageBucket, opts) {
		if obj.Err != nil {
			return fmt.Errorf("failed to list storage objects: %w", obj.Err)
		}
		// Skip directories/prefixes
		if strings.HasSuffix(obj.Key, "/") {
			continue
		}
		objects = append(objects, obj)
	}

	// Sort newest first
	slices.SortFunc(objects, func(a, b minio.ObjectInfo) int {
		if a.LastModified.After(b.LastModified) {
			return -1
		}
		if a.LastModified.Before(b.LastModified) {
			return 1
		}
		return 0
	})

	deleted := 0
	kept := 0

	for i, obj := range objects {
		if i < cfg.KeepLast {
			kept++
			continue
		}

		// Protect the newest verified backup (docs/DECISIONS.md D7)
		if cfg.ProtectedKey != "" && obj.Key == cfg.ProtectedKey {
			kept++
			continue
		}

		removeOpts := minio.RemoveObjectOptions{}
		if err := minioClient.RemoveObject(ctx, cfg.StorageBucket, obj.Key, removeOpts); err != nil {
			return fmt.Errorf("failed to remove object %s: %w", obj.Key, err)
		}
		deleted++
	}

	result := PruneResultMessage{
		Deleted: deleted,
		Kept:    kept,
	}

	if err := writeTerminationLog(cfg.TerminationLog, result); err != nil {
		return fmt.Errorf("failed to write termination log: %w", err)
	}

	return nil
}
