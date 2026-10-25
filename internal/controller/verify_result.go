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

package controller

import (
	"fmt"
	"math"
	"strings"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
	"github.com/souvik-biswas-dev/backupvouch/internal/checks"
)

// maxFailureMessage caps the text copied from a runner into a condition, an event and the status.
const maxFailureMessage = 300

// isValidVerifyFailureReason reports whether a reason written by the verify runner is one the controller trusts
// (docs/DECISIONS.md D11). Reasons the controller sets itself (SecretMissing, VerificationTimeout) are not listed
// because the runner never writes them.
func isValidVerifyFailureReason(reason string) bool {
	switch reason {
	case backupv1alpha1.ReasonChecksumMismatch,
		backupv1alpha1.ReasonRestoreFailed,
		backupv1alpha1.ReasonCheckFailed,
		backupv1alpha1.ReasonStorageUnreachable,
		backupv1alpha1.ReasonInvalidRunnerResult:
		return true
	default:
		return false
	}
}

// failedCheckNames returns the names of the checks that did not pass.
func failedCheckNames(results []checks.CheckResult) []string {
	var names []string
	for _, c := range results {
		if !c.Passed {
			names = append(names, c.Name)
		}
	}
	return names
}

// verifyFailureMessage builds the short text stored in lastVerification.message and the VerificationFailed event.
// Failed checks are described by name and by the runner's own message (which never carries row data). When no check
// ran, the text depends on the reason.
func verifyFailureMessage(reason string, results []checks.CheckResult) string {
	var parts []string
	for _, c := range results {
		if c.Passed {
			continue
		}
		if c.Message != "" {
			parts = append(parts, fmt.Sprintf("check %s failed: %s", c.Name, c.Message))
		} else {
			parts = append(parts, fmt.Sprintf("check %s failed", c.Name))
		}
	}
	if len(parts) > 0 {
		return truncateMessage(strings.Join(parts, "; "))
	}

	switch reason {
	case backupv1alpha1.ReasonChecksumMismatch:
		return "the stored backup does not match the checksum recorded when it was taken"
	case backupv1alpha1.ReasonStorageUnreachable:
		return "the backup object could not be downloaded from storage"
	case backupv1alpha1.ReasonRestoreFailed:
		return "the backup could not be restored into the scratch database"
	case backupv1alpha1.ReasonCheckFailed:
		return "a check could not be run against the restored database"
	case backupv1alpha1.ReasonInvalidRunnerResult:
		return "the verify runner could not use the checks it was given"
	default:
		return "restore verification failed"
	}
}

func truncateMessage(s string) string {
	r := []rune(s)
	if len(r) <= maxFailureMessage {
		return s
	}
	return string(r[:maxFailureMessage-1]) + "…"
}

// wholeSeconds turns the runner's fractional restore time into the whole seconds the CRD stores. It rounds up, so a
// restore that took 0.4 s shows as 1 s instead of "not recorded".
func wholeSeconds(f float64) int64 {
	if f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return int64(math.Ceil(f))
}
