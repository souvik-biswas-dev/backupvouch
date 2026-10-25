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
	"math"
	"slices"
	"strings"
	"testing"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
	"github.com/souvik-biswas-dev/backupvouch/internal/checks"
)

func TestIsValidVerifyFailureReason(t *testing.T) {
	valid := []string{
		backupv1alpha1.ReasonChecksumMismatch,
		backupv1alpha1.ReasonRestoreFailed,
		backupv1alpha1.ReasonCheckFailed,
		backupv1alpha1.ReasonStorageUnreachable,
		backupv1alpha1.ReasonInvalidRunnerResult,
	}
	for _, reason := range valid {
		if !isValidVerifyFailureReason(reason) {
			t.Errorf("%q should be trusted", reason)
		}
	}
	// SecretMissing and VerificationTimeout are set by the controller itself, never by the runner.
	for _, reason := range []string{"", "Bogus", "SecretMissing", "VerificationTimeout", "checksummismatch"} {
		if isValidVerifyFailureReason(reason) {
			t.Errorf("%q should not be trusted", reason)
		}
	}
}

func TestVerifyFailureMessage(t *testing.T) {
	results := []checks.CheckResult{
		{Name: "ok", Passed: true, Message: "fine"},
		{Name: "users-count", Passed: false, Message: `expected "3", got "2"`},
		{Name: "no-message", Passed: false},
	}

	got := verifyFailureMessage(backupv1alpha1.ReasonCheckFailed, results)
	want := `check users-count failed: expected "3", got "2"; check no-message failed`
	if got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
	if strings.Contains(got, "fine") {
		t.Errorf("a passed check must not appear in the message: %q", got)
	}

	if names := failedCheckNames(results); !slices.Equal(names, []string{"users-count", "no-message"}) {
		t.Errorf("failedCheckNames = %v", names)
	}

	for reason, fragment := range map[string]string{
		backupv1alpha1.ReasonChecksumMismatch:    "checksum",
		backupv1alpha1.ReasonStorageUnreachable:  "downloaded",
		backupv1alpha1.ReasonRestoreFailed:       "restored",
		backupv1alpha1.ReasonCheckFailed:         "could not be run",
		backupv1alpha1.ReasonInvalidRunnerResult: "checks it was given",
		"":                                       "verification failed",
	} {
		if msg := verifyFailureMessage(reason, nil); !strings.Contains(msg, fragment) {
			t.Errorf("reason %q: message %q does not contain %q", reason, msg, fragment)
		}
	}
}

func TestVerifyFailureMessageIsCapped(t *testing.T) {
	results := make([]checks.CheckResult, 0, 50)
	for range 50 {
		results = append(results, checks.CheckResult{Name: "a-long-check-name", Message: strings.Repeat("x", 50)})
	}
	msg := verifyFailureMessage(backupv1alpha1.ReasonCheckFailed, results)
	if n := len([]rune(msg)); n > maxFailureMessage {
		t.Errorf("message has %d characters, limit is %d", n, maxFailureMessage)
	}
	if !strings.HasSuffix(msg, "…") {
		t.Errorf("a cut message should end with an ellipsis: %q", msg)
	}
}

func TestWholeSeconds(t *testing.T) {
	for in, want := range map[float64]int64{
		-1: 0, 0: 0, 0.2: 1, 1: 1, 3.2: 4, 12.0: 12, math.NaN(): 0, math.Inf(1): 0,
	} {
		if got := wholeSeconds(in); got != want {
			t.Errorf("wholeSeconds(%v) = %d, want %d", in, got, want)
		}
	}
}
