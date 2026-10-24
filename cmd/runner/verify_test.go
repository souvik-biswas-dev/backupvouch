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
	"fmt"
	"os"
	"strings"
	"testing"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
	"github.com/souvik-biswas-dev/backupvouch/internal/checks"
)

func TestBuildVerifyResult(t *testing.T) {
	t.Run("normal message fits without truncation", func(t *testing.T) {
		checkResults := []checks.CheckResult{
			{Name: "check-1", Passed: true},
			{Name: "check-2", Passed: true},
		}

		res := BuildVerifyResult(backupv1alpha1.VerificationPassed, "", 12.34, checkResults)
		if res.Result != "Passed" || res.Reason != "" {
			t.Errorf("unexpected result/reason: %+v", res)
		}
		if res.ChecksTruncated {
			t.Errorf("ChecksTruncated should be false")
		}
		if len(res.Checks) != 2 {
			t.Errorf("len(res.Checks) = %d, expected 2", len(res.Checks))
		}

		data, err := json.Marshal(res)
		if err != nil {
			t.Fatalf("json marshal error: %v", err)
		}
		if len(data) > 3500 {
			t.Errorf("size %d exceeds 3500 bytes", len(data))
		}
	})

	t.Run("large message truncates passed checks and keeps failed checks", func(t *testing.T) {
		// Generate 50 passed checks and 2 failed checks with max allowed message sizes
		checkResults := make([]checks.CheckResult, 0)
		for i := range 40 {
			checkResults = append(checkResults, checks.CheckResult{
				Name:    fmt.Sprintf("passed-check-%d", i),
				Passed:  true,
				Message: strings.Repeat("ok-detail-", 10),
			})
		}
		checkResults = append(checkResults, checks.CheckResult{
			Name:    "failing-check-1",
			Passed:  false,
			Message: `expected "true", got "false"`,
		})
		checkResults = append(checkResults, checks.CheckResult{
			Name:    "failing-check-2",
			Passed:  false,
			Message: `expected "10", got "0"`,
		})

		res := BuildVerifyResult(backupv1alpha1.VerificationFailed, backupv1alpha1.ReasonCheckFailed, 4.5, checkResults)
		if !res.ChecksTruncated {
			t.Errorf("expected ChecksTruncated to be true for oversized message")
		}
		if len(res.Checks) != 2 {
			t.Errorf("expected only 2 failing checks retained, got %d", len(res.Checks))
		}
		for _, c := range res.Checks {
			if c.Passed {
				t.Errorf("found passed check in truncated result: %+v", c)
			}
		}

		data, err := json.Marshal(res)
		if err != nil {
			t.Fatalf("json marshal error: %v", err)
		}
		if len(data) > 3500 {
			t.Errorf("truncated size %d exceeds 3500 bytes", len(data))
		}
	})
}

func TestLoadVerifyConfig(t *testing.T) {
	t.Run("missing required env returns error", func(t *testing.T) {
		os.Clearenv()
		_, err := LoadVerifyConfig()
		if err == nil {
			t.Errorf("expected error when required env vars are missing")
		}
	})

	t.Run("all required env provided succeeds", func(t *testing.T) {
		t.Setenv("STORAGE_ENDPOINT", "minio:9000")
		t.Setenv("STORAGE_BUCKET", "backups")
		t.Setenv("STORAGE_ACCESS_KEY", "access")
		t.Setenv("STORAGE_SECRET_KEY", "secret")
		t.Setenv("OBJECT_KEY", "app/dump.dump")
		t.Setenv("EXPECTED_SHA256", "abc123hash")

		cfg, err := LoadVerifyConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.StorageBucket != "backups" || cfg.ExpectedSHA256 != "abc123hash" {
			t.Errorf("unexpected config loaded: %+v", cfg)
		}
		if cfg.DatabaseName != "app" || cfg.ScratchDir != "/scratch" {
			t.Errorf("unexpected defaults: %+v", cfg)
		}
	})
}
