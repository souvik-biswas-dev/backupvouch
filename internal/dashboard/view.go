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

// Package dashboard serves a small read-only status page and the JSON behind
// it. It never writes to the cluster and only reads BackupPolicy objects from
// the manager's cache (D21).
package dashboard

import (
	"cmp"
	"slices"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
	"github.com/souvik-biswas-dev/backupvouch/internal/schedule"
)

// Verdict answers one question about a policy: can the newest backup be trusted?
type Verdict string

// The four verdicts. Only a verification of the newest backup can produce
// VerdictVouched or VerdictFailed.
const (
	VerdictVouched      Verdict = "vouched"
	VerdictFailed       Verdict = "failed"
	VerdictBackupFailed Verdict = "backup-failed"
	VerdictUnverified   Verdict = "unverified"
)

// RunState is what is known about one past run.
type RunState string

// The four run states, drawn as four different shapes in the page.
const (
	RunVouched      RunState = "vouched"
	RunVerifyFailed RunState = "verify-failed"
	RunBackupFailed RunState = "backup-failed"
	RunUnverified   RunState = "unverified"
)

// Response is the body of GET /api/v1/policies.
type Response struct {
	GeneratedAt time.Time `json:"generatedAt"`
	Summary     Summary   `json:"summary"`
	Policies    []Policy  `json:"policies"`
}

// Summary counts policies by verdict.
type Summary struct {
	Total         int `json:"total"`
	Vouched       int `json:"vouched"`
	NeedAttention int `json:"needAttention"` // failed or backup-failed
	Unverified    int `json:"unverified"`
	InProgress    int `json:"inProgress"` // phase is not Idle
}

// Policy is the public view of one BackupPolicy. It deliberately leaves out
// the source host and database, the storage endpoint and bucket, and the
// names of the credential Secrets.
type Policy struct {
	Namespace        string        `json:"namespace"`
	Name             string        `json:"name"`
	Schedule         string        `json:"schedule"`
	TimeZone         string        `json:"timeZone,omitempty"`
	Suspended        bool          `json:"suspended"`
	Phase            string        `json:"phase"`
	Verdict          Verdict       `json:"verdict"`
	NextRun          *time.Time    `json:"nextRun,omitempty"`
	LastBackup       *Backup       `json:"lastBackup,omitempty"`
	LastVerification *Verification `json:"lastVerification,omitempty"`
	History          []Run         `json:"history"`
}

// Backup mirrors status.lastBackup.
type Backup struct {
	Time      *time.Time `json:"time,omitempty"`
	Result    string     `json:"result,omitempty"`
	ObjectKey string     `json:"objectKey,omitempty"`
	SizeBytes int64      `json:"sizeBytes,omitempty"`
	SHA256    string     `json:"sha256,omitempty"`
	Message   string     `json:"message,omitempty"`
}

// Verification mirrors status.lastVerification.
type Verification struct {
	Time                   *time.Time `json:"time,omitempty"`
	Result                 string     `json:"result,omitempty"`
	ObjectKey              string     `json:"objectKey,omitempty"`
	RestoreDurationSeconds int64      `json:"restoreDurationSeconds,omitempty"`
	FailedChecks           []string   `json:"failedChecks,omitempty"`
	Message                string     `json:"message,omitempty"`
}

// Run is one entry of status.history, oldest first in the response.
type Run struct {
	RunID              int64     `json:"runID"`
	At                 time.Time `json:"at"` // when the run finished, or the scheduled time if that is unknown
	ObjectKey          string    `json:"objectKey,omitempty"`
	BackupResult       string    `json:"backupResult,omitempty"`
	VerificationResult string    `json:"verificationResult,omitempty"`
	State              RunState  `json:"state"`
}

// BuildResponse converts BackupPolicy objects into the response. It is a pure
// function of its inputs, so it can be tested without a cluster.
func BuildResponse(items []backupv1alpha1.BackupPolicy, now time.Time) Response {
	resp := Response{
		GeneratedAt: now.UTC(),
		Policies:    make([]Policy, 0, len(items)),
	}
	for i := range items {
		p := buildPolicy(&items[i], now)
		resp.Policies = append(resp.Policies, p)
		resp.Summary.Total++
		switch p.Verdict {
		case VerdictVouched:
			resp.Summary.Vouched++
		case VerdictFailed, VerdictBackupFailed:
			resp.Summary.NeedAttention++
		case VerdictUnverified:
			resp.Summary.Unverified++
		}
		if p.Phase != string(backupv1alpha1.PhaseIdle) {
			resp.Summary.InProgress++
		}
	}
	sortPolicies(resp.Policies)
	return resp
}

func buildPolicy(bp *backupv1alpha1.BackupPolicy, now time.Time) Policy {
	st := &bp.Status
	phase := string(st.Phase)
	if phase == "" {
		phase = string(backupv1alpha1.PhaseIdle)
	}
	p := Policy{
		Namespace: bp.Namespace,
		Name:      bp.Name,
		Schedule:  bp.Spec.Schedule,
		TimeZone:  bp.Spec.TimeZone,
		Suspended: bp.Spec.Suspend,
		Phase:     phase,
		Verdict:   verdictOf(st),
		History:   buildHistory(st.History),
	}
	if !bp.Spec.Suspend {
		if next, err := schedule.NextRun(bp.Spec.Schedule, bp.Spec.TimeZone, now); err == nil {
			n := next.UTC()
			p.NextRun = &n
		}
	}
	if lb := st.LastBackup; lb != nil {
		p.LastBackup = &Backup{
			Time:      utc(lb.Time),
			Result:    string(lb.Result),
			ObjectKey: lb.ObjectKey,
			SizeBytes: lb.SizeBytes,
			SHA256:    lb.SHA256,
			Message:   lb.Message,
		}
	}
	if lv := st.LastVerification; lv != nil {
		p.LastVerification = &Verification{
			Time:                   utc(lv.Time),
			Result:                 string(lv.Result),
			ObjectKey:              lv.ObjectKey,
			RestoreDurationSeconds: lv.RestoreDurationSeconds,
			FailedChecks:           append([]string(nil), lv.FailedChecks...),
			Message:                lv.Message,
		}
	}
	return p
}

// verdictOf decides whether the NEWEST backup is vouched for. A verification of
// an older backup says nothing about the newest one.
func verdictOf(st *backupv1alpha1.BackupPolicyStatus) Verdict {
	lb, lv := st.LastBackup, st.LastVerification
	if lb != nil && lb.Result == backupv1alpha1.BackupFailed {
		return VerdictBackupFailed
	}
	if lv == nil {
		return VerdictUnverified
	}
	if lb != nil && lv.ObjectKey != lb.ObjectKey {
		return VerdictUnverified
	}
	switch lv.Result {
	case backupv1alpha1.VerificationFailed:
		return VerdictFailed
	case backupv1alpha1.VerificationPassed:
		return VerdictVouched
	default:
		return VerdictUnverified
	}
}

func runStateOf(r backupv1alpha1.RunRecord) RunState {
	switch {
	case r.BackupResult == backupv1alpha1.BackupFailed:
		return RunBackupFailed
	case r.VerificationResult == backupv1alpha1.VerificationFailed:
		return RunVerifyFailed
	case r.VerificationResult == backupv1alpha1.VerificationPassed:
		return RunVouched
	default:
		return RunUnverified
	}
}

// buildHistory returns the runs oldest first (status.history is newest first).
func buildHistory(in []backupv1alpha1.RunRecord) []Run {
	out := make([]Run, 0, len(in))
	for _, r := range in {
		at := time.Unix(r.RunID, 0).UTC()
		if r.Time != nil {
			at = r.Time.UTC()
		}
		out = append(out, Run{
			RunID:              r.RunID,
			At:                 at,
			ObjectKey:          r.ObjectKey,
			BackupResult:       string(r.BackupResult),
			VerificationResult: string(r.VerificationResult),
			State:              runStateOf(r),
		})
	}
	slices.SortStableFunc(out, func(a, b Run) int { return a.At.Compare(b.At) })
	return out
}

func utc(t *metav1.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

// attentionRank orders policies so that the ones needing a person come first.
func attentionRank(v Verdict) int {
	switch v {
	case VerdictFailed:
		return 0
	case VerdictBackupFailed:
		return 1
	case VerdictUnverified:
		return 2
	default:
		return 3
	}
}

func sortPolicies(ps []Policy) {
	slices.SortStableFunc(ps, func(a, b Policy) int {
		return cmp.Or(
			cmp.Compare(attentionRank(a.Verdict), attentionRank(b.Verdict)),
			cmp.Compare(a.Namespace, b.Namespace),
			cmp.Compare(a.Name, b.Name),
		)
	})
}
