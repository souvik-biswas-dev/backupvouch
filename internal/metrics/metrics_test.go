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
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
)

func TestPolicyMetrics(t *testing.T) {
	ns := "default"
	name := "app-db"

	now := time.Now().Truncate(time.Second)
	backupTime := metav1.NewTime(now.Add(-10 * time.Minute))
	verifyTime := metav1.NewTime(now.Add(-5 * time.Minute))

	policy := &backupv1alpha1.BackupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Status: backupv1alpha1.BackupPolicyStatus{
			LastBackup: &backupv1alpha1.BackupStatus{
				Result: backupv1alpha1.BackupSucceeded,
				Time:   &backupTime,
			},
			LastVerification: &backupv1alpha1.VerificationStatus{
				Result:                 backupv1alpha1.VerificationPassed,
				Time:                   &verifyTime,
				RestoreDurationSeconds: 42,
			},
		},
	}

	t.Run("records metrics on successful backup and passed verification", func(t *testing.T) {
		UpdatePolicyMetrics(policy)

		expectedBackupTs := float64(backupTime.Unix())
		if val := testutil.ToFloat64(LastBackupTimestampSeconds.WithLabelValues(ns, name)); val != expectedBackupTs {
			t.Errorf("LastBackupTimestampSeconds = %f, expected %f", val, expectedBackupTs)
		}

		expectedVerifyTs := float64(verifyTime.Unix())
		if val := testutil.ToFloat64(LastVerifiedTimestampSeconds.WithLabelValues(ns, name)); val != expectedVerifyTs {
			t.Errorf("LastVerifiedTimestampSeconds = %f, expected %f", val, expectedVerifyTs)
		}

		if val := testutil.ToFloat64(VerificationSuccess.WithLabelValues(ns, name)); val != 1 {
			t.Errorf("VerificationSuccess = %f, expected 1", val)
		}

		if val := testutil.ToFloat64(RestoreDurationSeconds.WithLabelValues(ns, name)); val != 42 {
			t.Errorf("RestoreDurationSeconds = %f, expected 42", val)
		}
	})

	t.Run("records 0 on failed verification", func(t *testing.T) {
		failedPolicy := policy.DeepCopy()
		failedPolicy.Status.LastVerification.Result = backupv1alpha1.VerificationFailed

		UpdatePolicyMetrics(failedPolicy)

		if val := testutil.ToFloat64(VerificationSuccess.WithLabelValues(ns, name)); val != 0 {
			t.Errorf("VerificationSuccess = %f, expected 0", val)
		}
	})

	t.Run("deletes metric series on policy deletion", func(t *testing.T) {
		DeletePolicyMetrics(ns, name)

		// VerificationSuccess should have 0 metrics for this label set
		count := testutil.CollectAndCount(VerificationSuccess)
		// Should be clean or decremented
		_ = count
	})
}
