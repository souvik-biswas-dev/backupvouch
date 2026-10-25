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
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
	"github.com/souvik-biswas-dev/backupvouch/internal/jobs"
	"github.com/souvik-biswas-dev/backupvouch/internal/metrics"
)

// These specs add idempotency and failure paths to the happy path in backuppolicy_controller_test.go.
// envtest runs no Jobs, so each spec sets Job and Pod status by hand and calls Reconcile directly.

const (
	failureTestNamespace = "default"
	backupResultOK       = `{"objectKey":"app/2026-10-24T02-00-00Z.dump","sizeBytes":1234,"sha256":"abc"}`
)

func newFailureTestPolicy(name string) *backupv1alpha1.BackupPolicy {
	return &backupv1alpha1.BackupPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: failureTestNamespace},
		Spec: backupv1alpha1.BackupPolicySpec{
			Schedule: "0 2 * * *",
			TimeZone: "UTC",
			Source: backupv1alpha1.SourceSpec{
				Host:                 "postgres.default.svc",
				Database:             "app",
				CredentialsSecretRef: backupv1alpha1.SecretReference{Name: "pg-creds"},
			},
			Storage: backupv1alpha1.StorageSpec{
				Endpoint:             "minio.default.svc:9000",
				Bucket:               "backups",
				CredentialsSecretRef: backupv1alpha1.SecretReference{Name: "minio-creds"},
			},
		},
	}
}

func reconcileTimes(ctx context.Context, r *BackupPolicyReconciler, key types.NamespacedName, n int) {
	GinkgoHelper()
	for range n {
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
	}
}

func fetchPolicy(ctx context.Context, key types.NamespacedName) *backupv1alpha1.BackupPolicy {
	GinkgoHelper()
	policy := &backupv1alpha1.BackupPolicy{}
	Expect(k8sClient.Get(ctx, key, policy)).To(Succeed())
	return policy
}

func fetchJob(ctx context.Context, name string) *batchv1.Job {
	GinkgoHelper()
	job := &batchv1.Job{}
	Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: failureTestNamespace, Name: name}, job)).To(Succeed())
	return job
}

func countJobsWithRole(ctx context.Context, policyName, role string) int {
	GinkgoHelper()
	var list batchv1.JobList
	Expect(k8sClient.List(ctx, &list,
		client.InNamespace(failureTestNamespace),
		client.MatchingLabels{
			backupv1alpha1.LabelPolicy: policyName,
			backupv1alpha1.LabelRole:   role,
		},
	)).To(Succeed())
	return len(list.Items)
}

// markJobFailed sets a Failed Job status. Recent Kubernetes versions validate Job status strictly, so the
// Failed condition comes with FailureTarget and a start time. VERIFY: accepted by the envtest API server.
func markJobFailed(ctx context.Context, job *batchv1.Job, reason string) {
	GinkgoHelper()
	now := metav1.Now()
	start := metav1.NewTime(now.Add(-10 * time.Second))
	job.Status.StartTime = &start
	job.Status.Conditions = []batchv1.JobCondition{
		{Type: batchv1.JobFailureTarget, Status: corev1.ConditionTrue, Reason: reason, LastTransitionTime: now},
		{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: reason, LastTransitionTime: now},
	}
	Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
}

func createJobPod(ctx context.Context, jobName string, state corev1.ContainerState) {
	GinkgoHelper()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName + "-pod",
			Namespace: failureTestNamespace,
			Labels:    map[string]string{"batch.kubernetes.io/job-name": jobName},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: jobs.RunnerContainerName, Image: "runner"}},
		},
	}
	Expect(k8sClient.Create(ctx, pod)).To(Succeed())
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: jobs.RunnerContainerName, State: state}}
	Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
}

func terminatedPod(ctx context.Context, jobName, message string, exitCode int32) {
	GinkgoHelper()
	createJobPod(ctx, jobName, corev1.ContainerState{
		Terminated: &corev1.ContainerStateTerminated{ExitCode: exitCode, Message: message},
	})
}

func expectCondition(
	policy *backupv1alpha1.BackupPolicy,
	condType string,
	status metav1.ConditionStatus,
	reason string,
) {
	GinkgoHelper()
	cond := meta.FindStatusCondition(policy.Status.Conditions, condType)
	Expect(cond).NotTo(BeNil(), "condition %s is missing", condType)
	Expect(cond.Status).To(Equal(status), "condition %s: %s", condType, cond.Message)
	Expect(cond.Reason).To(Equal(reason), "condition %s: %s", condType, cond.Message)
}

var _ = Describe("BackupPolicy controller failure paths", func() {
	ctx := context.Background()
	fixedTime := time.Date(2026, 10, 24, 2, 5, 0, 0, time.UTC)

	var (
		reconciler *BackupPolicyReconciler
		name       string
		key        types.NamespacedName
	)

	// start creates the policy and drives it to BackingUp. It returns the run ID.
	start := func(policyName string) int64 {
		GinkgoHelper()
		name = policyName
		key = types.NamespacedName{Namespace: failureTestNamespace, Name: name}
		Expect(k8sClient.Create(ctx, newFailureTestPolicy(name))).To(Succeed())
		reconcileTimes(ctx, reconciler, key, 2) // add the finalizer, then start the run
		policy := fetchPolicy(ctx, key)
		Expect(policy.Status.Phase).To(Equal(backupv1alpha1.PhaseBackingUp))
		return policy.Status.CurrentRunID
	}

	// startVerifying drives the policy through a successful backup into the Verifying phase.
	startVerifying := func(policyName string) int64 {
		GinkgoHelper()
		runID := start(policyName)
		backupJobName := jobs.BackupJobName(name, runID)
		Expect(markJobComplete(ctx, fetchJob(ctx, backupJobName))).To(Succeed())
		terminatedPod(ctx, backupJobName, backupResultOK, 0)
		reconcileTimes(ctx, reconciler, key, 1)
		Expect(fetchPolicy(ctx, key).Status.Phase).To(Equal(backupv1alpha1.PhaseVerifying))
		return runID
	}

	BeforeEach(func() {
		name = ""
		reconciler = &BackupPolicyReconciler{
			Client:  k8sClient,
			Scheme:  k8sClient.Scheme(),
			NowFunc: func() time.Time { return fixedTime },
		}
	})

	AfterEach(func() {
		if name == "" {
			return
		}
		policy := &backupv1alpha1.BackupPolicy{}
		if err := k8sClient.Get(ctx, key, policy); err == nil {
			_ = k8sClient.Delete(ctx, policy)
			// Reconcile once more so the finalizer runs.
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		}
	})

	Context("operator restart", func() {
		It("brings the metrics back from status on the next reconcile", func() {
			startVerifying("metrics-restart")
			metrics.DeletePolicyMetrics(failureTestNamespace, name) // what a restarted operator looks like

			reconcileTimes(ctx, reconciler, key, 1)

			Expect(testutil.ToFloat64(metrics.LastBackupTimestampSeconds.WithLabelValues(failureTestNamespace, name))).
				To(Equal(float64(fixedTime.Unix())))
		})
	})

	Context("idempotency", func() {
		It("creates exactly one backup Job however often reconcile runs", func() {
			runID := start("idem-policy")
			reconcileTimes(ctx, reconciler, key, 3)
			Expect(countJobsWithRole(ctx, name, backupv1alpha1.RoleBackup)).To(Equal(1))

			By("skipping a schedule that arrives while the run is in progress")
			later := fixedTime.Add(25 * time.Hour)
			reconciler.NowFunc = func() time.Time { return later }
			reconcileTimes(ctx, reconciler, key, 2)
			policy := fetchPolicy(ctx, key)
			Expect(policy.Status.Phase).To(Equal(backupv1alpha1.PhaseBackingUp))
			Expect(policy.Status.CurrentRunID).To(Equal(runID))
			Expect(countJobsWithRole(ctx, name, backupv1alpha1.RoleBackup)).To(Equal(1))

			By("recovering from a crash between creating the Job and recording it in status")
			reconciler.NowFunc = func() time.Time { return fixedTime }
			policy.Status.Phase = backupv1alpha1.PhaseIdle
			policy.Status.CurrentRunID = 0
			policy.Status.LastRunID = 0
			Expect(k8sClient.Status().Update(ctx, policy)).To(Succeed())
			reconcileTimes(ctx, reconciler, key, 1)
			policy = fetchPolicy(ctx, key)
			Expect(policy.Status.CurrentRunID).To(Equal(runID))
			Expect(countJobsWithRole(ctx, name, backupv1alpha1.RoleBackup)).To(Equal(1))
		})
	})

	Context("backup failures", func() {
		It("reports the reason from the runner's error message", func() {
			runID := start("fail-source")
			jobName := jobs.BackupJobName(name, runID)
			markJobFailed(ctx, fetchJob(ctx, jobName), "BackoffLimitExceeded")
			terminatedPod(ctx, jobName,
				`{"error":{"reason":"SourceUnreachable","message":"could not connect to host"}}`, 1)

			reconcileTimes(ctx, reconciler, key, 1)

			policy := fetchPolicy(ctx, key)
			Expect(policy.Status.Phase).To(Equal(backupv1alpha1.PhaseIdle))
			Expect(policy.Status.CurrentRunID).To(BeZero())
			Expect(policy.Status.LastBackup).NotTo(BeNil())
			Expect(policy.Status.LastBackup.Result).To(Equal(backupv1alpha1.BackupFailed))
			expectCondition(policy, backupv1alpha1.ConditionBackupReady,
				metav1.ConditionFalse, backupv1alpha1.ReasonSourceUnreachable)
			Expect(policy.Status.History).To(HaveLen(1))
			Expect(countJobsWithRole(ctx, name, backupv1alpha1.RoleVerify)).To(BeZero())
			Expect(countJobsWithRole(ctx, name, backupv1alpha1.RolePrune)).To(BeZero())
		})

		It("reports BackupTimeout when the Job exceeds its deadline", func() {
			runID := start("fail-backup-timeout")
			markJobFailed(ctx, fetchJob(ctx, jobs.BackupJobName(name, runID)), "DeadlineExceeded")

			reconcileTimes(ctx, reconciler, key, 1)

			policy := fetchPolicy(ctx, key)
			Expect(policy.Status.Phase).To(Equal(backupv1alpha1.PhaseIdle))
			expectCondition(policy, backupv1alpha1.ConditionBackupReady,
				metav1.ConditionFalse, backupv1alpha1.ReasonBackupTimeout)
		})

		It("reports InvalidRunnerResult when a succeeded Job left no usable message", func() {
			runID := start("fail-backup-message")
			jobName := jobs.BackupJobName(name, runID)
			Expect(markJobComplete(ctx, fetchJob(ctx, jobName))).To(Succeed())
			terminatedPod(ctx, jobName, "this is not json", 0)

			reconcileTimes(ctx, reconciler, key, 1)

			policy := fetchPolicy(ctx, key)
			Expect(policy.Status.Phase).To(Equal(backupv1alpha1.PhaseIdle))
			expectCondition(policy, backupv1alpha1.ConditionBackupReady,
				metav1.ConditionFalse, backupv1alpha1.ReasonInvalidRunnerResult)
			Expect(countJobsWithRole(ctx, name, backupv1alpha1.RoleVerify)).To(BeZero())
		})

		It("fails with SecretMissing when the Pod cannot start for a missing Secret, and removes the Job", func() {
			runID := start("fail-secret")
			jobName := jobs.BackupJobName(name, runID)
			createJobPod(ctx, jobName, corev1.ContainerState{
				Waiting: &corev1.ContainerStateWaiting{Reason: "CreateContainerConfigError"},
			})

			// The controller waits 60 seconds after the Pod was created before it gives up.
			reconciler.NowFunc = func() time.Time { return time.Now().Add(2 * time.Minute) }
			reconcileTimes(ctx, reconciler, key, 1)

			policy := fetchPolicy(ctx, key)
			Expect(policy.Status.Phase).To(Equal(backupv1alpha1.PhaseIdle))
			expectCondition(policy, backupv1alpha1.ConditionBackupReady,
				metav1.ConditionFalse, backupv1alpha1.ReasonSecretMissing)
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: failureTestNamespace, Name: jobName}, &batchv1.Job{})
			Expect(errors.IsNotFound(err)).To(BeTrue(), "the stuck Job should be deleted")
		})
	})

	Context("verification failures", func() {
		It("records ChecksumMismatch, keeps BackupReady true and does not prune", func() {
			runID := startVerifying("fail-checksum")
			jobName := jobs.VerifyJobName(name, runID)
			markJobFailed(ctx, fetchJob(ctx, jobName), "BackoffLimitExceeded")
			terminatedPod(ctx, jobName, `{"result":"Failed","reason":"ChecksumMismatch"}`, 1)

			reconcileTimes(ctx, reconciler, key, 1)

			policy := fetchPolicy(ctx, key)
			Expect(policy.Status.Phase).To(Equal(backupv1alpha1.PhaseIdle))
			Expect(policy.Status.LastVerification).NotTo(BeNil())
			Expect(policy.Status.LastVerification.Result).To(Equal(backupv1alpha1.VerificationFailed))
			Expect(policy.Status.LastVerification.ObjectKey).To(Equal("app/2026-10-24T02-00-00Z.dump"))
			expectCondition(policy, backupv1alpha1.ConditionRestoreVerified,
				metav1.ConditionFalse, backupv1alpha1.ReasonChecksumMismatch)
			expectCondition(policy, backupv1alpha1.ConditionBackupReady,
				metav1.ConditionTrue, backupv1alpha1.ReasonBackupSucceeded)
			Expect(countJobsWithRole(ctx, name, backupv1alpha1.RolePrune)).To(BeZero(),
				"retention must not run after a failed verification")
		})

		It("names the failed checks on CheckFailed", func() {
			runID := startVerifying("fail-check")
			jobName := jobs.VerifyJobName(name, runID)
			markJobFailed(ctx, fetchJob(ctx, jobName), "BackoffLimitExceeded")
			terminatedPod(ctx, jobName,
				`{"result":"Failed","reason":"CheckFailed","restoreDurationSeconds":3.2,`+
					`"checks":[{"name":"users-not-empty","passed":false,"message":"expected true"},`+
					`{"name":"other","passed":true}]}`, 1)

			reconcileTimes(ctx, reconciler, key, 1)

			policy := fetchPolicy(ctx, key)
			expectCondition(policy, backupv1alpha1.ConditionRestoreVerified,
				metav1.ConditionFalse, backupv1alpha1.ReasonCheckFailed)
			Expect(policy.Status.LastVerification.FailedChecks).To(ConsistOf("users-not-empty"))
			Expect(policy.Status.LastVerification.Message).To(Equal("check users-not-empty failed: expected true"))
			Expect(policy.Status.LastVerification.RestoreDurationSeconds).To(Equal(int64(4)),
				"3.2 s is rounded up, and a failed run still records how long the restore took")
		})

		It("gives a checksum failure a readable message", func() {
			runID := startVerifying("fail-checksum-message")
			jobName := jobs.VerifyJobName(name, runID)
			markJobFailed(ctx, fetchJob(ctx, jobName), "BackoffLimitExceeded")
			terminatedPod(ctx, jobName, `{"result":"Failed","reason":"ChecksumMismatch"}`, 1)

			reconcileTimes(ctx, reconciler, key, 1)

			policy := fetchPolicy(ctx, key)
			Expect(policy.Status.LastVerification.Message).To(ContainSubstring("checksum"))
			Expect(policy.Status.LastVerification.FailedChecks).To(BeEmpty())
		})

		It("does not copy an unknown reason from the runner into the condition", func() {
			runID := startVerifying("fail-unknown-reason")
			jobName := jobs.VerifyJobName(name, runID)
			markJobFailed(ctx, fetchJob(ctx, jobName), "BackoffLimitExceeded")
			terminatedPod(ctx, jobName, `{"result":"Failed","reason":"TotallyMadeUp"}`, 1)

			reconcileTimes(ctx, reconciler, key, 1)

			expectCondition(fetchPolicy(ctx, key), backupv1alpha1.ConditionRestoreVerified,
				metav1.ConditionFalse, backupv1alpha1.ReasonRestoreFailed)
		})

		It("recreates a verify Job that was deleted while Verifying", func() {
			runID := startVerifying("verify-job-deleted")
			jobName := jobs.VerifyJobName(name, runID)
			Expect(k8sClient.Delete(ctx, fetchJob(ctx, jobName),
				client.PropagationPolicy(metav1.DeletePropagationBackground))).To(Succeed())

			reconcileTimes(ctx, reconciler, key, 1)

			Expect(fetchJob(ctx, jobName).Labels[backupv1alpha1.LabelRole]).To(Equal(backupv1alpha1.RoleVerify))
			Expect(fetchPolicy(ctx, key).Status.Phase).To(Equal(backupv1alpha1.PhaseVerifying))
			Expect(countJobsWithRole(ctx, name, backupv1alpha1.RoleVerify)).To(Equal(1))
		})

		It("reports VerificationTimeout when the Job exceeds its deadline", func() {
			runID := startVerifying("fail-verify-timeout")
			markJobFailed(ctx, fetchJob(ctx, jobs.VerifyJobName(name, runID)), "DeadlineExceeded")

			reconcileTimes(ctx, reconciler, key, 1)

			policy := fetchPolicy(ctx, key)
			Expect(policy.Status.Phase).To(Equal(backupv1alpha1.PhaseIdle))
			expectCondition(policy, backupv1alpha1.ConditionRestoreVerified,
				metav1.ConditionFalse, backupv1alpha1.ReasonVerificationTimeout)
		})

		It("reports InvalidRunnerResult when the verification message cannot be parsed", func() {
			runID := startVerifying("fail-verify-message")
			jobName := jobs.VerifyJobName(name, runID)
			Expect(markJobComplete(ctx, fetchJob(ctx, jobName))).To(Succeed())
			terminatedPod(ctx, jobName, "this is not json", 0)

			reconcileTimes(ctx, reconciler, key, 1)

			policy := fetchPolicy(ctx, key)
			expectCondition(policy, backupv1alpha1.ConditionRestoreVerified,
				metav1.ConditionFalse, backupv1alpha1.ReasonInvalidRunnerResult)
		})
	})
})
