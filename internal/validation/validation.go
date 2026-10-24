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
	"fmt"
	"path/filepath"
	"strings"
	"time"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
	"github.com/souvik-biswas-dev/backupvouch/internal/schedule"
)

// ValidationResult represents the outcome of validating a BackupPolicy spec.
type ValidationResult struct {
	Valid   bool
	Reason  string
	Message string
}

// ValidateSpec checks schedule cron format, IANA time zone, and runner image allow-listing per D8 & D14.
func ValidateSpec(
	policy *backupv1alpha1.BackupPolicy,
	allowedImagePatterns []string,
	defaultRunnerImage string,
) ValidationResult {
	if policy == nil {
		return ValidationResult{
			Valid:   false,
			Reason:  "NilPolicy",
			Message: "policy is nil",
		}
	}

	// 1. Validate TimeZone
	tz := policy.Spec.TimeZone
	if tz == "" {
		tz = "UTC"
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return ValidationResult{
			Valid:   false,
			Reason:  backupv1alpha1.ReasonInvalidTimeZone,
			Message: fmt.Sprintf("invalid time zone %q: %v", policy.Spec.TimeZone, err),
		}
	}

	// 2. Validate Schedule cron expression
	if _, _, err := schedule.Parse(policy.Spec.Schedule, tz); err != nil {
		return ValidationResult{
			Valid:   false,
			Reason:  backupv1alpha1.ReasonInvalidSchedule,
			Message: fmt.Sprintf("invalid schedule cron %q: %v", policy.Spec.Schedule, err),
		}
	}

	// 3. Validate RunnerImage against allow-list (D14)
	if !IsRunnerImageAllowed(policy.Spec.RunnerImage, allowedImagePatterns, defaultRunnerImage) {
		return ValidationResult{
			Valid:   false,
			Reason:  backupv1alpha1.ReasonRunnerImageNotAllowed,
			Message: fmt.Sprintf("runner image %q is not in the operator allow-list", policy.Spec.RunnerImage),
		}
	}

	return ValidationResult{
		Valid:   true,
		Reason:  backupv1alpha1.ReasonSpecValid,
		Message: "spec is valid",
	}
}

// IsRunnerImageAllowed determines whether the specified image is permitted to execute.
// If empty, it defaults to defaultRunnerImage and is allowed.
func IsRunnerImageAllowed(image string, allowedPatterns []string, defaultRunnerImage string) bool {
	if image == "" || image == defaultRunnerImage {
		return true
	}

	for _, pattern := range allowedPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		// Try exact match or glob pattern match
		if pattern == image {
			return true
		}
		if matched, err := filepath.Match(pattern, image); err == nil && matched {
			return true
		}
	}

	return false
}
