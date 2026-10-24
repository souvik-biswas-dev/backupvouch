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

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
)

const (
	labelNamespace = "namespace"
	labelPolicy    = "policy"
)

var (
	// LastBackupTimestampSeconds tracks the finish time of the last successful backup.
	LastBackupTimestampSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "backupvouch_last_backup_timestamp_seconds",
			Help: "Finish timestamp (Unix seconds) of the last successful backup.",
		},
		[]string{labelNamespace, labelPolicy},
	)

	// LastVerifiedTimestampSeconds tracks the finish time of the last passed verification.
	LastVerifiedTimestampSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "backupvouch_last_verified_timestamp_seconds",
			Help: "Finish timestamp (Unix seconds) of the last passed verification.",
		},
		[]string{labelNamespace, labelPolicy},
	)

	// VerificationSuccess tracks the outcome of the last verification (1=passed, 0=failed).
	VerificationSuccess = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "backupvouch_verification_success",
			Help: "Result of the last verification (1=passed, 0=failed).",
		},
		[]string{labelNamespace, labelPolicy},
	)

	// RestoreDurationSeconds tracks the measured duration of the last completed restore.
	RestoreDurationSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "backupvouch_restore_duration_seconds",
			Help: "Measured duration in whole seconds of the last completed restore.",
		},
		[]string{labelNamespace, labelPolicy},
	)
)

func init() {
	crmetrics.Registry.MustRegister(
		LastBackupTimestampSeconds,
		LastVerifiedTimestampSeconds,
		VerificationSuccess,
		RestoreDurationSeconds,
	)
}

// UpdatePolicyMetrics updates Prometheus gauge vectors based on the observed policy status.
func UpdatePolicyMetrics(policy *backupv1alpha1.BackupPolicy) {
	if policy == nil {
		return
	}

	ns := policy.Namespace
	name := policy.Name

	// 1. Last backup timestamp
	if policy.Status.LastBackup != nil &&
		policy.Status.LastBackup.Result == backupv1alpha1.BackupSucceeded &&
		policy.Status.LastBackup.Time != nil {
		ts := float64(policy.Status.LastBackup.Time.Unix())
		LastBackupTimestampSeconds.WithLabelValues(ns, name).Set(ts)
	}

	// 2. Verification metrics
	if policy.Status.LastVerification != nil {
		lv := policy.Status.LastVerification

		switch lv.Result {
		case backupv1alpha1.VerificationPassed:
			VerificationSuccess.WithLabelValues(ns, name).Set(1)
			if lv.Time != nil {
				ts := float64(lv.Time.Unix())
				LastVerifiedTimestampSeconds.WithLabelValues(ns, name).Set(ts)
			}
		case backupv1alpha1.VerificationFailed:
			VerificationSuccess.WithLabelValues(ns, name).Set(0)
		}

		if lv.RestoreDurationSeconds > 0 {
			RestoreDurationSeconds.WithLabelValues(ns, name).Set(float64(lv.RestoreDurationSeconds))
		}
	}
}

// DeletePolicyMetrics cleans up all metric series associated with a policy when it is deleted.
func DeletePolicyMetrics(namespace, policyName string) {
	LastBackupTimestampSeconds.DeleteLabelValues(namespace, policyName)
	LastVerifiedTimestampSeconds.DeleteLabelValues(namespace, policyName)
	VerificationSuccess.DeleteLabelValues(namespace, policyName)
	RestoreDurationSeconds.DeleteLabelValues(namespace, policyName)
}
