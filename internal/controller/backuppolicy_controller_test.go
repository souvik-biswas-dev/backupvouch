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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
	"github.com/souvik-biswas-dev/backupvouch/internal/jobs"
)

var _ = Describe("BackupPolicy Controller", func() {
	Context("When reconciling a resource", func() {
		const (
			resourceName      = "test-resource"
			resourceNamespace = "default"
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: resourceNamespace,
		}

		var controllerReconciler *BackupPolicyReconciler
		fixedTime := time.Date(2026, 10, 24, 2, 5, 0, 0, time.UTC)

		BeforeEach(func() {
			controllerReconciler = &BackupPolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				NowFunc: func() time.Time {
					return fixedTime
				},
			}
		})

		AfterEach(func() {
			policy := &backupv1alpha1.BackupPolicy{}
			if err := k8sClient.Get(ctx, typeNamespacedName, policy); err == nil {
				_ = k8sClient.Delete(ctx, policy)
				// Reconcile deletion to process finalizer
				_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			}
		})

		It("validates spec and sets SpecValid=False on invalid schedule", func() {
			invalidPolicy := &backupv1alpha1.BackupPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: backupv1alpha1.BackupPolicySpec{
					Schedule: "99 99 * * *",
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
			Expect(k8sClient.Create(ctx, invalidPolicy)).To(Succeed())

			// 1. Add finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			// 2. Validate spec
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			fetched := &backupv1alpha1.BackupPolicy{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, fetched)).To(Succeed())

			cond := fetched.Status.Conditions
			Expect(cond).ToNot(BeEmpty())
			specValidCond := cond[0]
			Expect(specValidCond.Type).To(Equal(backupv1alpha1.ConditionSpecValid))
			Expect(specValidCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(specValidCond.Reason).To(Equal(backupv1alpha1.ReasonInvalidSchedule))
		})

		It("executes the full reconciliation state machine", func() {
			policy := &backupv1alpha1.BackupPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
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
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			// Step 1: Adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			// Step 2: Validates spec, checks schedule (due at 02:00:00), starts backup
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			fetched := &backupv1alpha1.BackupPolicy{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(backupv1alpha1.PhaseBackingUp))
			expectedRunID := time.Date(2026, 10, 24, 2, 0, 0, 0, time.UTC).Unix()
			Expect(fetched.Status.CurrentRunID).To(Equal(expectedRunID))

			// Check backup Job was created
			backupJobName := jobs.BackupJobName(resourceName, expectedRunID)
			backupJob := &batchv1.Job{}
			backupJobKey := client.ObjectKey{Namespace: resourceNamespace, Name: backupJobName}
			Expect(k8sClient.Get(ctx, backupJobKey, backupJob)).To(Succeed())

			// Mock backup Job success and Pod termination log
			Expect(markJobComplete(ctx, backupJob)).To(Succeed())

			mockBackupPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      backupJobName + "-pod",
					Namespace: resourceNamespace,
					Labels:    map[string]string{"batch.kubernetes.io/job-name": backupJobName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "runner", Image: "runner"}},
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "runner",
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode: 0,
									Message:  `{"objectKey":"app/2026-10-24T02-00-00Z.dump","sizeBytes":1234,"sha256":"abc"}`,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, mockBackupPod)).To(Succeed())
			mockBackupPod.Status.ContainerStatuses = []corev1.ContainerStatus{
				{
					Name: "runner",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 0,
							Message:  `{"objectKey":"app/2026-10-24T02-00-00Z.dump","sizeBytes":1234,"sha256":"abc"}`,
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(ctx, mockBackupPod)).To(Succeed())

			// Step 3: Reconcile BackingUp -> completes backup -> transitions to Verifying
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, typeNamespacedName, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(backupv1alpha1.PhaseVerifying))
			Expect(fetched.Status.LastBackup.Result).To(Equal(backupv1alpha1.BackupSucceeded))

			// Check verify Job was created
			verifyJobName := jobs.VerifyJobName(resourceName, expectedRunID)
			verifyJobKey := client.ObjectKey{Namespace: resourceNamespace, Name: verifyJobName}
			verifyJob := &batchv1.Job{}
			Expect(k8sClient.Get(ctx, verifyJobKey, verifyJob)).To(Succeed())

			// Mock verify Job success and Pod termination log
			Expect(markJobComplete(ctx, verifyJob)).To(Succeed())

			mockVerifyPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      verifyJobName + "-pod",
					Namespace: resourceNamespace,
					Labels:    map[string]string{"batch.kubernetes.io/job-name": verifyJobName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "runner", Image: "runner"}},
				},
			}
			Expect(k8sClient.Create(ctx, mockVerifyPod)).To(Succeed())
			mockVerifyPod.Status.ContainerStatuses = []corev1.ContainerStatus{
				{
					Name: "runner",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 0,
							Message:  `{"result":"Passed","reason":"","restoreDurationSeconds":4.2,"checks":[]}`,
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(ctx, mockVerifyPod)).To(Succeed())

			// Step 4: Reconcile Verifying -> completes verification -> transitions to Pruning
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, typeNamespacedName, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(backupv1alpha1.PhasePruning))
			Expect(fetched.Status.LastVerification.Result).To(Equal(backupv1alpha1.VerificationPassed))
			Expect(fetched.Status.LastVerification.RestoreDurationSeconds).To(Equal(int64(4)))

			// Trigger reconcilePruning to create prune Job
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			// Check prune Job was created
			pruneJobName := jobs.PruneJobName(resourceName, expectedRunID)
			pruneJobKey := client.ObjectKey{Namespace: resourceNamespace, Name: pruneJobName}
			pruneJob := &batchv1.Job{}
			Expect(k8sClient.Get(ctx, pruneJobKey, pruneJob)).To(Succeed())

			// Mock prune Job success
			Expect(markJobComplete(ctx, pruneJob)).To(Succeed())

			// Step 5: Reconcile Pruning -> completes pruning -> transitions to Idle
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, typeNamespacedName, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(backupv1alpha1.PhaseIdle))
			Expect(fetched.Status.CurrentRunID).To(Equal(int64(0)))
			Expect(fetched.Status.History).To(HaveLen(1))
			Expect(fetched.Status.History[0].BackupResult).To(Equal(backupv1alpha1.BackupSucceeded))
			Expect(fetched.Status.History[0].VerificationResult).To(Equal(backupv1alpha1.VerificationPassed))

			// Step 6: Test manual re-verify annotation
			reverifyTimestamp := int64(1771910000)
			fetched.Annotations = map[string]string{
				backupv1alpha1.AnnotationReverify: "1771910000",
			}
			Expect(k8sClient.Update(ctx, fetched)).To(Succeed())

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, typeNamespacedName, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(backupv1alpha1.PhaseVerifying))
			Expect(fetched.Status.LastReverifyID).To(Equal(reverifyTimestamp))

			// Step 7: Test finalizer deletes owned Jobs on policy deletion
			Expect(k8sClient.Delete(ctx, fetched)).To(Succeed())
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			// Verify policy is removed
			deletedPolicy := &backupv1alpha1.BackupPolicy{}
			err = k8sClient.Get(ctx, typeNamespacedName, deletedPolicy)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})
	})
})

func markJobComplete(ctx context.Context, job *batchv1.Job) error {
	nowMeta := metav1.Now()
	startMeta := metav1.NewTime(nowMeta.Add(-10 * time.Second))
	job.Status.StartTime = &startMeta
	job.Status.CompletionTime = &nowMeta
	job.Status.Conditions = []batchv1.JobCondition{
		{
			Type:   batchv1.JobSuccessCriteriaMet,
			Status: corev1.ConditionTrue,
		},
		{
			Type:   batchv1.JobComplete,
			Status: corev1.ConditionTrue,
		},
	}
	return k8sClient.Status().Update(ctx, job)
}
