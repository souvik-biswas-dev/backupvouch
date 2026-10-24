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

package validation

import (
	"testing"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
)

func TestValidateSpec(t *testing.T) {
	defaultImage := "ghcr.io/backupvouch/runner:v0.1.0"
	allowedPatterns := []string{
		"ghcr.io/myorg/*",
		"internal-registry.corp/runner:16",
	}

	tests := []struct {
		name         string
		policy       *backupv1alpha1.BackupPolicy
		wantValid    bool
		wantReason   string
	}{
		{
			name: "valid standard policy",
			policy: &backupv1alpha1.BackupPolicy{
				Spec: backupv1alpha1.BackupPolicySpec{
					Schedule: "0 2 * * *",
					TimeZone: "America/New_York",
				},
			},
			wantValid:  true,
			wantReason: backupv1alpha1.ReasonSpecValid,
		},
		{
			name: "invalid timezone",
			policy: &backupv1alpha1.BackupPolicy{
				Spec: backupv1alpha1.BackupPolicySpec{
					Schedule: "0 2 * * *",
					TimeZone: "Mars/Olympus_Mons",
				},
			},
			wantValid:  false,
			wantReason: backupv1alpha1.ReasonInvalidTimeZone,
		},
		{
			name: "invalid cron syntax",
			policy: &backupv1alpha1.BackupPolicy{
				Spec: backupv1alpha1.BackupPolicySpec{
					Schedule: "not a cron",
					TimeZone: "UTC",
				},
			},
			wantValid:  false,
			wantReason: backupv1alpha1.ReasonInvalidSchedule,
		},
		{
			name: "allowed custom runner image by glob pattern",
			policy: &backupv1alpha1.BackupPolicy{
				Spec: backupv1alpha1.BackupPolicySpec{
					Schedule:    "0 2 * * *",
					TimeZone:    "UTC",
					RunnerImage: "ghcr.io/myorg/custom-runner:v1.2",
				},
			},
			wantValid:  true,
			wantReason: backupv1alpha1.ReasonSpecValid,
		},
		{
			name: "disallowed custom runner image",
			policy: &backupv1alpha1.BackupPolicy{
				Spec: backupv1alpha1.BackupPolicySpec{
					Schedule:    "0 2 * * *",
					TimeZone:    "UTC",
					RunnerImage: "evil/untrusted:latest",
				},
			},
			wantValid:  false,
			wantReason: backupv1alpha1.ReasonRunnerImageNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := ValidateSpec(tt.policy, allowedPatterns, defaultImage)
			if res.Valid != tt.wantValid {
				t.Errorf("Valid = %v, expected %v", res.Valid, tt.wantValid)
			}
			if res.Reason != tt.wantReason {
				t.Errorf("Reason = %q, expected %q", res.Reason, tt.wantReason)
			}
		})
	}
}

func TestIsRunnerImageAllowed(t *testing.T) {
	defaultImage := "ghcr.io/backupvouch/runner:v0.1.0"
	patterns := []string{"ghcr.io/acme/*", "example.com/repo/image:v1"}

	if !IsRunnerImageAllowed("", patterns, defaultImage) {
		t.Errorf("empty image should be allowed (uses default)")
	}
	if !IsRunnerImageAllowed(defaultImage, patterns, defaultImage) {
		t.Errorf("default image should be allowed")
	}
	if !IsRunnerImageAllowed("ghcr.io/acme/postgres:16", patterns, defaultImage) {
		t.Errorf("image matching glob should be allowed")
	}
	if IsRunnerImageAllowed("ghcr.io/other/postgres:16", patterns, defaultImage) {
		t.Errorf("image not matching any pattern should be rejected")
	}
}
