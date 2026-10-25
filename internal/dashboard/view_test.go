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

package dashboard

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
)

func mustTime(t *testing.T, s string) *metav1.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("bad test time %q: %v", s, err)
	}
	mt := metav1.NewTime(parsed)
	return &mt
}

func newPolicy(name string, mutate func(*backupv1alpha1.BackupPolicy)) backupv1alpha1.BackupPolicy {
	bp := backupv1alpha1.BackupPolicy{}
	bp.Namespace = "default"
	bp.Name = name
	bp.Spec.Schedule = "0 2 * * *"
	if mutate != nil {
		mutate(&bp)
	}
	return bp
}

func TestVerdictOf(t *testing.T) {
	cases := []struct {
		name   string
		status backupv1alpha1.BackupPolicyStatus
		want   Verdict
	}{
		{
			name: "nothing has run",
			want: VerdictUnverified,
		},
		{
			name: "backup failed, older verification passed",
			status: backupv1alpha1.BackupPolicyStatus{
				LastBackup:       &backupv1alpha1.BackupStatus{Result: backupv1alpha1.BackupFailed},
				LastVerification: &backupv1alpha1.VerificationStatus{Result: backupv1alpha1.VerificationPassed, ObjectKey: "a"},
			},
			want: VerdictBackupFailed,
		},
		{
			name: "newest backup verified",
			status: backupv1alpha1.BackupPolicyStatus{
				LastBackup:       &backupv1alpha1.BackupStatus{Result: backupv1alpha1.BackupSucceeded, ObjectKey: "a"},
				LastVerification: &backupv1alpha1.VerificationStatus{Result: backupv1alpha1.VerificationPassed, ObjectKey: "a"},
			},
			want: VerdictVouched,
		},
		{
			name: "newest backup failed its checks",
			status: backupv1alpha1.BackupPolicyStatus{
				LastBackup:       &backupv1alpha1.BackupStatus{Result: backupv1alpha1.BackupSucceeded, ObjectKey: "a"},
				LastVerification: &backupv1alpha1.VerificationStatus{Result: backupv1alpha1.VerificationFailed, ObjectKey: "a"},
			},
			want: VerdictFailed,
		},
		{
			name: "verification belongs to an older backup",
			status: backupv1alpha1.BackupPolicyStatus{
				LastBackup:       &backupv1alpha1.BackupStatus{Result: backupv1alpha1.BackupSucceeded, ObjectKey: "b"},
				LastVerification: &backupv1alpha1.VerificationStatus{Result: backupv1alpha1.VerificationPassed, ObjectKey: "a"},
			},
			want: VerdictUnverified,
		},
		{
			name: "older failed verification does not condemn the newest backup",
			status: backupv1alpha1.BackupPolicyStatus{
				LastBackup:       &backupv1alpha1.BackupStatus{Result: backupv1alpha1.BackupSucceeded, ObjectKey: "b"},
				LastVerification: &backupv1alpha1.VerificationStatus{Result: backupv1alpha1.VerificationFailed, ObjectKey: "a"},
			},
			want: VerdictUnverified,
		},
		{
			name: "verification without a backup record, passed",
			status: backupv1alpha1.BackupPolicyStatus{
				LastVerification: &backupv1alpha1.VerificationStatus{Result: backupv1alpha1.VerificationPassed},
			},
			want: VerdictVouched,
		},
		{
			name: "verification without a backup record, failed",
			status: backupv1alpha1.BackupPolicyStatus{
				LastVerification: &backupv1alpha1.VerificationStatus{Result: backupv1alpha1.VerificationFailed},
			},
			want: VerdictFailed,
		},
		{
			name: "verification with an empty result",
			status: backupv1alpha1.BackupPolicyStatus{
				LastBackup:       &backupv1alpha1.BackupStatus{Result: backupv1alpha1.BackupSucceeded, ObjectKey: "a"},
				LastVerification: &backupv1alpha1.VerificationStatus{ObjectKey: "a"},
			},
			want: VerdictUnverified,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := verdictOf(&tc.status); got != tc.want {
				t.Fatalf("verdictOf() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRunStateOf(t *testing.T) {
	cases := []struct {
		name string
		rec  backupv1alpha1.RunRecord
		want RunState
	}{
		{"backup failed wins over a stale verification", backupv1alpha1.RunRecord{
			BackupResult: backupv1alpha1.BackupFailed, VerificationResult: backupv1alpha1.VerificationPassed,
		}, RunBackupFailed},
		{"verification failed", backupv1alpha1.RunRecord{
			BackupResult: backupv1alpha1.BackupSucceeded, VerificationResult: backupv1alpha1.VerificationFailed,
		}, RunVerifyFailed},
		{"verification passed", backupv1alpha1.RunRecord{
			BackupResult: backupv1alpha1.BackupSucceeded, VerificationResult: backupv1alpha1.VerificationPassed,
		}, RunVouched},
		{"backup only", backupv1alpha1.RunRecord{BackupResult: backupv1alpha1.BackupSucceeded}, RunUnverified},
		{"empty record", backupv1alpha1.RunRecord{}, RunUnverified},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runStateOf(tc.rec); got != tc.want {
				t.Fatalf("runStateOf() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildHistoryIsOldestFirstAndFallsBackToRunID(t *testing.T) {
	in := []backupv1alpha1.RunRecord{
		{RunID: 300, Time: mustTime(t, "2026-10-25T02:10:00Z")}, // newest first, as the controller stores it
		{RunID: 200}, // no finish time: falls back to the run ID
		{RunID: 100, Time: mustTime(t, "2026-10-23T02:10:00Z")},
	}
	got := buildHistory(in)
	if len(got) != 3 {
		t.Fatalf("got %d runs, want 3", len(got))
	}
	// RunID 200 is 1970, so it sorts first; the others keep their order.
	wantIDs := []int64{200, 100, 300}
	for i, id := range wantIDs {
		if got[i].RunID != id {
			t.Fatalf("run %d has ID %d, want %d (order %+v)", i, got[i].RunID, id, got)
		}
	}
	if !got[0].At.Equal(time.Unix(200, 0)) {
		t.Fatalf("fallback time = %v, want the run ID as a Unix time", got[0].At)
	}
}

func TestBuildResponseOrderAndSummary(t *testing.T) {
	now := time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	vouched := newPolicy("a-vouched", func(bp *backupv1alpha1.BackupPolicy) {
		bp.Status.LastBackup = &backupv1alpha1.BackupStatus{Result: backupv1alpha1.BackupSucceeded, ObjectKey: "k"}
		bp.Status.LastVerification = &backupv1alpha1.VerificationStatus{Result: backupv1alpha1.VerificationPassed, ObjectKey: "k"}
	})
	failed := newPolicy("z-failed", func(bp *backupv1alpha1.BackupPolicy) {
		bp.Status.LastBackup = &backupv1alpha1.BackupStatus{Result: backupv1alpha1.BackupSucceeded, ObjectKey: "k"}
		bp.Status.LastVerification = &backupv1alpha1.VerificationStatus{Result: backupv1alpha1.VerificationFailed, ObjectKey: "k"}
		bp.Status.Phase = backupv1alpha1.PhaseVerifying
	})
	backupFailed := newPolicy("m-backup-failed", func(bp *backupv1alpha1.BackupPolicy) {
		bp.Status.LastBackup = &backupv1alpha1.BackupStatus{Result: backupv1alpha1.BackupFailed}
	})
	fresh := newPolicy("b-fresh", nil)

	resp := BuildResponse([]backupv1alpha1.BackupPolicy{vouched, fresh, backupFailed, failed}, now)

	order := make([]string, 0, len(resp.Policies))
	for _, p := range resp.Policies {
		order = append(order, p.Name)
	}
	want := []string{"z-failed", "m-backup-failed", "b-fresh", "a-vouched"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", order, want)
	}
	wantSummary := Summary{Total: 4, Vouched: 1, NeedAttention: 2, Unverified: 1, InProgress: 1}
	if resp.Summary != wantSummary {
		t.Fatalf("summary = %+v, want %+v", resp.Summary, wantSummary)
	}
	if !resp.GeneratedAt.Equal(now) {
		t.Fatalf("generatedAt = %v, want %v", resp.GeneratedAt, now)
	}
	for _, p := range resp.Policies {
		if p.Phase == "" {
			t.Fatalf("policy %s has an empty phase", p.Name)
		}
	}
}

func TestNextRun(t *testing.T) {
	now := time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		mutate func(*backupv1alpha1.BackupPolicy)
		want   *time.Time
	}{
		{"daily in UTC", nil, new(time.Date(2026, 10, 26, 2, 0, 0, 0, time.UTC))},
		{"suspended policy has no next run", func(bp *backupv1alpha1.BackupPolicy) { bp.Spec.Suspend = true }, nil},
		{"invalid schedule has no next run", func(bp *backupv1alpha1.BackupPolicy) { bp.Spec.Schedule = "not a cron" }, nil},
		{"invalid time zone has no next run", func(bp *backupv1alpha1.BackupPolicy) { bp.Spec.TimeZone = "Mars/Olympus" }, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := BuildResponse([]backupv1alpha1.BackupPolicy{newPolicy("p", tc.mutate)}, now)
			got := resp.Policies[0].NextRun
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("NextRun = %v, want none", *got)
			case tc.want != nil && (got == nil || !got.Equal(*tc.want)):
				t.Fatalf("NextRun = %v, want %v", got, *tc.want)
			}
		})
	}
}

func TestResponseDoesNotLeakConnectionDetails(t *testing.T) {
	bp := newPolicy("leaky", func(bp *backupv1alpha1.BackupPolicy) {
		bp.Spec.Source.Host = "db.internal.example"
		bp.Spec.Source.Database = "customers-prod"
		bp.Spec.Source.CredentialsSecretRef.Name = "source-secret-name"
		bp.Spec.Storage.Endpoint = "s3.internal.example:9000"
		bp.Spec.Storage.Bucket = "bucket-name-xyz"
		bp.Spec.Storage.CredentialsSecretRef.Name = "storage-secret-name"
	})
	raw, err := json.Marshal(BuildResponse([]backupv1alpha1.BackupPolicy{bp}, time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{
		"db.internal.example", "customers-prod", "source-secret-name",
		"s3.internal.example", "bucket-name-xyz", "storage-secret-name",
	} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("response leaks %q: %s", secret, raw)
		}
	}
}

func TestEmptyResponseUsesEmptyArrays(t *testing.T) {
	raw, err := json.Marshal(BuildResponse(nil, time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"policies":[]`) {
		t.Fatalf("policies should be an empty array, got %s", raw)
	}
	withPolicy, err := json.Marshal(BuildResponse([]backupv1alpha1.BackupPolicy{newPolicy("p", nil)}, time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(withPolicy), `"history":[]`) {
		t.Fatalf("history should be an empty array, got %s", withPolicy)
	}
}
