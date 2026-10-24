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
	"fmt"
	"math"
	"slices"
	"strconv"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/recorder"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
	"github.com/souvik-biswas-dev/backupvouch/internal/jobs"
	"github.com/souvik-biswas-dev/backupvouch/internal/metrics"
	"github.com/souvik-biswas-dev/backupvouch/internal/schedule"
	"github.com/souvik-biswas-dev/backupvouch/internal/validation"
)

// BackupPolicyReconciler reconciles a BackupPolicy object
type BackupPolicyReconciler struct {
	client.Client
	Scheme                     *runtime.Scheme
	Recorder                   recorder.EventRecorder
	AllowedRunnerImagePatterns []string
	DefaultRunnerImage         string
	NowFunc                    func() time.Time
}

// +kubebuilder:rbac:groups=backup.backupvouch.dev,resources=backuppolicies,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=backup.backupvouch.dev,resources=backuppolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=backup.backupvouch.dev,resources=backuppolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// VERIFY: the recorder comes from mgr.GetEventRecorder (events.k8s.io API), so the rule is for events.k8s.io, not core events.
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch;update

// Reconcile executes the state machine per docs/ARCHITECTURE.md.
func (r *BackupPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var policy backupv1alpha1.BackupPolicy
	if err := r.Get(ctx, req.NamespacedName, &policy); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	now := r.now()

	// 1. Handle finalizer
	if !policy.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &policy)
	}
	if !controllerutil.ContainsFinalizer(&policy, backupv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(&policy, backupv1alpha1.FinalizerName)
		if err := r.Update(ctx, &policy); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
		return ctrl.Result{RequeueAfter: time.Millisecond}, nil
	}

	// 2. Validate Spec (SpecValid condition)
	valRes := validation.ValidateSpec(&policy, r.AllowedRunnerImagePatterns, r.DefaultRunnerImage)
	r.setCondition(
		&policy,
		backupv1alpha1.ConditionSpecValid,
		boolToConditionStatus(valRes.Valid),
		valRes.Reason,
		valRes.Message,
	)
	if !valRes.Valid {
		if err := r.Status().Update(ctx, &policy); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update status with invalid spec: %w", err)
		}
		log.Info("BackupPolicy spec is invalid, halting runs", "reason", valRes.Reason)
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	// 3. Suspend check
	if policy.Spec.Suspend {
		if policy.Status.Phase == "" || policy.Status.Phase == backupv1alpha1.PhaseIdle {
			return ctrl.Result{}, r.Status().Update(ctx, &policy)
		}
	}

	// 4. State machine dispatch
	switch policy.Status.Phase {
	case "", backupv1alpha1.PhaseIdle:
		return r.reconcileIdle(ctx, &policy, now)
	case backupv1alpha1.PhaseBackingUp:
		return r.reconcileBackingUp(ctx, &policy, now)
	case backupv1alpha1.PhaseVerifying:
		return r.reconcileVerifying(ctx, &policy, now)
	case backupv1alpha1.PhasePruning:
		return r.reconcilePruning(ctx, &policy, now)
	default:
		policy.Status.Phase = backupv1alpha1.PhaseIdle
		return ctrl.Result{RequeueAfter: time.Millisecond}, r.Status().Update(ctx, &policy)
	}
}

func (r *BackupPolicyReconciler) reconcileIdle(
	ctx context.Context,
	policy *backupv1alpha1.BackupPolicy,
	now time.Time,
) (ctrl.Result, error) {
	// Initialize conditions if missing
	r.initConditions(policy)

	// Check manual re-verify annotation (docs/DECISIONS.md D13)
	if valStr, ok := policy.Annotations[backupv1alpha1.AnnotationReverify]; ok {
		reverifyID, err := strconv.ParseInt(valStr, 10, 64)
		if err == nil && reverifyID > 0 && reverifyID != policy.Status.LastReverifyID {
			if policy.Status.LastBackup != nil &&
				policy.Status.LastBackup.Result == backupv1alpha1.BackupSucceeded {
				return r.startReverify(ctx, policy, reverifyID)
			}
		}
	}

	// Check schedule
	isDue, nextScheduled, err := schedule.IsDue(
		policy.Spec.Schedule,
		policy.Spec.TimeZone,
		policy.Status.LastRunID,
		now,
	)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to evaluate schedule: %w", err)
	}

	if isDue {
		runID := nextScheduled.Unix()
		policy.Status.Phase = backupv1alpha1.PhaseBackingUp
		policy.Status.CurrentRunID = runID
		policy.Status.LastRunID = runID
		r.emitEvent(policy, corev1.EventTypeNormal, "BackupStarted", "Started scheduled backup")

		backupJob, err := jobs.BuildBackupJob(policy, runID)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to build backup job: %w", err)
		}
		if err := r.Create(ctx, backupJob); client.IgnoreAlreadyExists(err) != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create backup job: %w", err)
		}

		if err := r.Status().Update(ctx, policy); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update status to BackingUp: %w", err)
		}
		return ctrl.Result{RequeueAfter: time.Millisecond}, nil
	}

	// Not due: requeue for next run
	nextRun, err := schedule.NextRun(policy.Spec.Schedule, policy.Spec.TimeZone, now)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to compute next run: %w", err)
	}

	requeueDuration := max(nextRun.Sub(now), time.Second)

	policy.Status.Phase = backupv1alpha1.PhaseIdle
	policy.Status.CurrentRunID = 0
	if err := r.Status().Update(ctx, policy); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{RequeueAfter: requeueDuration}, nil
}

func (r *BackupPolicyReconciler) reconcileBackingUp(
	ctx context.Context,
	policy *backupv1alpha1.BackupPolicy,
	now time.Time,
) (ctrl.Result, error) {
	jobName := jobs.BackupJobName(policy.Name, policy.Status.CurrentRunID)
	var job batchv1.Job
	if err := r.Get(ctx, client.ObjectKey{Namespace: policy.Namespace, Name: jobName}, &job); err != nil {
		if errors.IsNotFound(err) {
			// Recreate if missing (idempotency)
			bJob, buildErr := jobs.BuildBackupJob(policy, policy.Status.CurrentRunID)
			if buildErr != nil {
				return ctrl.Result{}, buildErr
			}
			if cErr := r.Create(ctx, bJob); client.IgnoreAlreadyExists(cErr) != nil {
				return ctrl.Result{}, cErr
			}
			return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}

	// Check missing secret
	inspection, _ := InspectJobPods(ctx, r.Client, policy.Namespace, jobName, now)
	if inspection.SecretMissing {
		return r.failBackup(
			ctx,
			policy,
			backupv1alpha1.ReasonSecretMissing,
			"Secret referenced by backup job is missing or invalid",
			&job,
		)
	}

	// Check job completion
	complete, success, deadlineExceeded := getJobCompletionStatus(&job)
	if !complete {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if !success {
		reason := backupv1alpha1.ReasonBackupJobFailed
		msg := "backup job failed"
		if deadlineExceeded {
			reason = backupv1alpha1.ReasonBackupTimeout
			msg = "backup job exceeded activeDeadlineSeconds"
		} else if parsed, err := ParseBackupResult(inspection.TerminationMessage); err == nil && parsed.Error != nil {
			if isValidBackupFailureReason(parsed.Error.Reason) {
				reason = parsed.Error.Reason
			}
			if parsed.Error.Message != "" {
				msg = parsed.Error.Message
			}
		} else if inspection.OOMKilled {
			msg = "backup container was OOMKilled"
		}
		return r.failBackup(ctx, policy, reason, msg, nil)
	}

	// Job Succeeded
	parsed, err := ParseBackupResult(inspection.TerminationMessage)
	if err != nil {
		return r.failBackup(
			ctx,
			policy,
			backupv1alpha1.ReasonInvalidRunnerResult,
			"backup job succeeded but termination message could not be parsed",
			nil,
		)
	}

	nowMeta := metav1.NewTime(now)
	policy.Status.LastBackup = &backupv1alpha1.BackupStatus{
		Time:      &nowMeta,
		Result:    backupv1alpha1.BackupSucceeded,
		ObjectKey: parsed.ObjectKey,
		SizeBytes: parsed.SizeBytes,
		SHA256:    parsed.SHA256,
		Message:   "",
	}
	r.setCondition(
		policy,
		backupv1alpha1.ConditionBackupReady,
		metav1.ConditionTrue,
		backupv1alpha1.ReasonBackupSucceeded,
		"backup completed successfully",
	)
	r.emitEvent(policy, corev1.EventTypeNormal, "BackupSucceeded", "Backup created successfully")
	metrics.UpdatePolicyMetrics(policy)

	// Verification dispatch
	if policy.Spec.Verify.IsEnabled() {
		policy.Status.Phase = backupv1alpha1.PhaseVerifying
		r.emitEvent(policy, corev1.EventTypeNormal, "VerificationStarted", "Started verification job")

		vJob, err := jobs.BuildVerifyJob(
			policy,
			policy.Status.CurrentRunID,
			parsed.ObjectKey,
			parsed.SHA256,
		)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to build verify job: %w", err)
		}
		if err := r.Create(ctx, vJob); client.IgnoreAlreadyExists(err) != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create verify job: %w", err)
		}
		return ctrl.Result{RequeueAfter: time.Millisecond}, r.Status().Update(ctx, policy)
	}

	// Verification disabled
	r.setCondition(
		policy,
		backupv1alpha1.ConditionRestoreVerified,
		metav1.ConditionUnknown,
		backupv1alpha1.ReasonVerificationDisabled,
		"verification is disabled",
	)
	policy.Status.Phase = backupv1alpha1.PhasePruning
	return ctrl.Result{RequeueAfter: time.Millisecond}, r.Status().Update(ctx, policy)
}

func (r *BackupPolicyReconciler) reconcileVerifying(
	ctx context.Context,
	policy *backupv1alpha1.BackupPolicy,
	now time.Time,
) (ctrl.Result, error) {
	jobName := jobs.VerifyJobName(policy.Name, policy.Status.CurrentRunID)
	var job batchv1.Job
	if err := r.Get(ctx, client.ObjectKey{Namespace: policy.Namespace, Name: jobName}, &job); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}

	inspection, _ := InspectJobPods(ctx, r.Client, policy.Namespace, jobName, now)
	if inspection.SecretMissing {
		return r.failVerify(
			ctx,
			policy,
			backupv1alpha1.ReasonSecretMissing,
			"Secret referenced by verify job is missing or invalid",
			nil,
			&job,
		)
	}

	complete, success, deadlineExceeded := getJobCompletionStatus(&job)
	if !complete {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	nowMeta := metav1.NewTime(now)
	objectKey := ""
	if policy.Status.LastBackup != nil {
		objectKey = policy.Status.LastBackup.ObjectKey
	}

	if deadlineExceeded {
		return r.failVerify(
			ctx,
			policy,
			backupv1alpha1.ReasonVerificationTimeout,
			"verification job exceeded activeDeadlineSeconds",
			nil,
			nil,
		)
	}

	parsed, err := ParseVerifyResult(inspection.TerminationMessage)
	if err != nil {
		reason := backupv1alpha1.ReasonInvalidRunnerResult
		msg := "verification termination message could not be parsed"
		if !success && inspection.OOMKilled {
			msg = "verify container was OOMKilled"
		}
		return r.failVerify(ctx, policy, reason, msg, nil, nil)
	}

	if parsed.Result == backupv1alpha1.VerificationPassed {
		policy.Status.LastVerification = &backupv1alpha1.VerificationStatus{
			Time:                   &nowMeta,
			Result:                 backupv1alpha1.VerificationPassed,
			ObjectKey:              objectKey,
			RestoreDurationSeconds: int64(math.Round(parsed.RestoreDurationSeconds)),
			FailedChecks:           nil,
			Message:                "",
		}
		r.setCondition(
			policy,
			backupv1alpha1.ConditionRestoreVerified,
			metav1.ConditionTrue,
			backupv1alpha1.ReasonVerificationPassed,
			"restore verification passed all checks",
		)
		r.emitEvent(policy, corev1.EventTypeNormal, "VerificationPassed", "Verification passed successfully")
		metrics.UpdatePolicyMetrics(policy)

		policy.Status.Phase = backupv1alpha1.PhasePruning
		return ctrl.Result{RequeueAfter: time.Millisecond}, r.Status().Update(ctx, policy)
	}

	// Verification failed
	var failedCheckNames []string
	for _, c := range parsed.Checks {
		if !c.Passed {
			failedCheckNames = append(failedCheckNames, c.Name)
		}
	}

	reason := parsed.Reason
	if reason == "" {
		reason = backupv1alpha1.ReasonRestoreFailed
	}
	return r.failVerify(
		ctx,
		policy,
		reason,
		"one or more checks or restore operations failed",
		failedCheckNames,
		nil,
	)
}

func (r *BackupPolicyReconciler) reconcilePruning(
	ctx context.Context,
	policy *backupv1alpha1.BackupPolicy,
	_ time.Time,
) (ctrl.Result, error) {
	protectedKey := ""
	if policy.Status.LastVerification != nil &&
		policy.Status.LastVerification.Result == backupv1alpha1.VerificationPassed {
		protectedKey = policy.Status.LastVerification.ObjectKey
	}

	jobName := jobs.PruneJobName(policy.Name, policy.Status.CurrentRunID)
	var job batchv1.Job
	if err := r.Get(ctx, client.ObjectKey{Namespace: policy.Namespace, Name: jobName}, &job); err != nil {
		if errors.IsNotFound(err) {
			pJob, buildErr := jobs.BuildPruneJob(policy, policy.Status.CurrentRunID, protectedKey)
			if buildErr != nil {
				return ctrl.Result{}, buildErr
			}
			if cErr := r.Create(ctx, pJob); client.IgnoreAlreadyExists(cErr) != nil {
				return ctrl.Result{}, cErr
			}
			return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}

	complete, success, _ := getJobCompletionStatus(&job)
	if !complete {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if success {
		r.emitEvent(policy, corev1.EventTypeNormal, "RetentionPruned", "Retention pruning completed successfully")
	} else {
		r.emitEvent(policy, corev1.EventTypeWarning, "RetentionFailed", "Retention pruning failed")
	}

	// Record history
	r.recordHistory(policy)

	// Clean up older Jobs (keep last 3 runs, docs/ARCHITECTURE.md line 86)
	_ = r.cleanupOldJobs(ctx, policy)

	policy.Status.Phase = backupv1alpha1.PhaseIdle
	policy.Status.CurrentRunID = 0
	if err := r.Status().Update(ctx, policy); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Millisecond}, nil
}

func (r *BackupPolicyReconciler) startReverify(
	ctx context.Context,
	policy *backupv1alpha1.BackupPolicy,
	reverifyID int64,
) (ctrl.Result, error) {
	policy.Status.LastReverifyID = reverifyID
	policy.Status.CurrentRunID = reverifyID
	policy.Status.Phase = backupv1alpha1.PhaseVerifying

	r.emitEvent(policy, corev1.EventTypeNormal, "VerificationStarted", "Started manual re-verification")

	vJob, err := jobs.BuildVerifyJob(
		policy,
		reverifyID,
		policy.Status.LastBackup.ObjectKey,
		policy.Status.LastBackup.SHA256,
	)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to build reverify job: %w", err)
	}
	if err := r.Create(ctx, vJob); client.IgnoreAlreadyExists(err) != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create reverify job: %w", err)
	}

	if err := r.Status().Update(ctx, policy); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: time.Millisecond}, nil
}

func (r *BackupPolicyReconciler) failBackup(
	ctx context.Context,
	policy *backupv1alpha1.BackupPolicy,
	reason, msg string,
	jobToDelete *batchv1.Job,
) (ctrl.Result, error) {
	if jobToDelete != nil {
		// Background propagation: batch/v1 Jobs default to orphaning their Pods, which would leave a stuck Pod behind.
		_ = r.Delete(ctx, jobToDelete, client.PropagationPolicy(metav1.DeletePropagationBackground))
	}

	nowMeta := metav1.NewTime(r.now())
	policy.Status.LastBackup = &backupv1alpha1.BackupStatus{
		Time:    &nowMeta,
		Result:  backupv1alpha1.BackupFailed,
		Message: msg,
	}
	r.setCondition(policy, backupv1alpha1.ConditionBackupReady, metav1.ConditionFalse, reason, msg)
	r.emitEvent(policy, corev1.EventTypeWarning, "BackupFailed", msg)
	metrics.UpdatePolicyMetrics(policy)

	r.recordHistory(policy)
	policy.Status.Phase = backupv1alpha1.PhaseIdle
	policy.Status.CurrentRunID = 0

	return ctrl.Result{}, r.Status().Update(ctx, policy)
}

func (r *BackupPolicyReconciler) failVerify(
	ctx context.Context,
	policy *backupv1alpha1.BackupPolicy,
	reason, msg string,
	failedChecks []string,
	jobToDelete *batchv1.Job,
) (ctrl.Result, error) {
	if jobToDelete != nil {
		// Background propagation: batch/v1 Jobs default to orphaning their Pods, which would leave a stuck Pod behind.
		_ = r.Delete(ctx, jobToDelete, client.PropagationPolicy(metav1.DeletePropagationBackground))
	}

	nowMeta := metav1.NewTime(r.now())
	objectKey := ""
	if policy.Status.LastBackup != nil {
		objectKey = policy.Status.LastBackup.ObjectKey
	}

	policy.Status.LastVerification = &backupv1alpha1.VerificationStatus{
		Time:         &nowMeta,
		Result:       backupv1alpha1.VerificationFailed,
		ObjectKey:    objectKey,
		FailedChecks: failedChecks,
		Message:      msg,
	}
	r.setCondition(policy, backupv1alpha1.ConditionRestoreVerified, metav1.ConditionFalse, reason, msg)
	r.emitEvent(policy, corev1.EventTypeWarning, "VerificationFailed", msg)
	metrics.UpdatePolicyMetrics(policy)

	r.recordHistory(policy)
	policy.Status.Phase = backupv1alpha1.PhaseIdle
	policy.Status.CurrentRunID = 0

	return ctrl.Result{}, r.Status().Update(ctx, policy)
}

func (r *BackupPolicyReconciler) handleDeletion(
	ctx context.Context,
	policy *backupv1alpha1.BackupPolicy,
) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(policy, backupv1alpha1.FinalizerName) {
		// Clean up owned Jobs
		var jobList batchv1.JobList
		opts := []client.ListOption{
			client.InNamespace(policy.Namespace),
			client.MatchingLabels{backupv1alpha1.LabelPolicy: policy.Name},
		}
		if err := r.List(ctx, &jobList, opts...); err == nil {
			for i := range jobList.Items {
				_ = r.Delete(ctx, &jobList.Items[i], client.PropagationPolicy(metav1.DeletePropagationBackground))
			}
		}

		// Clean up Prometheus metrics
		metrics.DeletePolicyMetrics(policy.Namespace, policy.Name)

		controllerutil.RemoveFinalizer(policy, backupv1alpha1.FinalizerName)
		if err := r.Update(ctx, policy); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}
	return ctrl.Result{}, nil
}

func (r *BackupPolicyReconciler) recordHistory(policy *backupv1alpha1.BackupPolicy) {
	nowMeta := metav1.NewTime(r.now())
	runRec := backupv1alpha1.RunRecord{
		RunID: policy.Status.CurrentRunID,
		Time:  &nowMeta,
	}
	if policy.Status.LastBackup != nil {
		runRec.ObjectKey = policy.Status.LastBackup.ObjectKey
		runRec.BackupResult = policy.Status.LastBackup.Result
	}
	if policy.Status.LastVerification != nil {
		runRec.VerificationResult = policy.Status.LastVerification.Result
	}

	history := append([]backupv1alpha1.RunRecord{runRec}, policy.Status.History...)
	if len(history) > 10 {
		history = history[:10]
	}
	policy.Status.History = history
}

func (r *BackupPolicyReconciler) cleanupOldJobs(ctx context.Context, policy *backupv1alpha1.BackupPolicy) error {
	var jobList batchv1.JobList
	opts := []client.ListOption{
		client.InNamespace(policy.Namespace),
		client.MatchingLabels{backupv1alpha1.LabelPolicy: policy.Name},
	}
	if err := r.List(ctx, &jobList, opts...); err != nil {
		return err
	}

	runIDsMap := make(map[int64]bool)
	for i := range jobList.Items {
		runIDStr := jobList.Items[i].Labels[backupv1alpha1.LabelRunID]
		if runID, err := strconv.ParseInt(runIDStr, 10, 64); err == nil {
			runIDsMap[runID] = true
		}
	}

	var runIDs []int64
	for id := range runIDsMap {
		runIDs = append(runIDs, id)
	}
	slices.SortFunc(runIDs, func(a, b int64) int {
		if a > b {
			return -1
		}
		if a < b {
			return 1
		}
		return 0
	})

	if len(runIDs) <= 3 {
		return nil
	}

	keepRunIDs := make(map[int64]bool)
	for _, id := range runIDs[:3] {
		keepRunIDs[id] = true
	}

	for i := range jobList.Items {
		j := &jobList.Items[i]
		runIDStr := j.Labels[backupv1alpha1.LabelRunID]
		if runID, err := strconv.ParseInt(runIDStr, 10, 64); err == nil {
			if !keepRunIDs[runID] {
				_ = r.Delete(ctx, j, client.PropagationPolicy(metav1.DeletePropagationBackground))
			}
		}
	}
	return nil
}

func (r *BackupPolicyReconciler) now() time.Time {
	if r.NowFunc != nil {
		return r.NowFunc()
	}
	return time.Now()
}

func (r *BackupPolicyReconciler) setCondition(
	policy *backupv1alpha1.BackupPolicy,
	condType string,
	status metav1.ConditionStatus,
	reason, message string,
) {
	meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: policy.Generation,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(r.now()),
	})
}

func (r *BackupPolicyReconciler) initConditions(policy *backupv1alpha1.BackupPolicy) {
	if meta.FindStatusCondition(policy.Status.Conditions, backupv1alpha1.ConditionBackupReady) == nil {
		r.setCondition(
			policy,
			backupv1alpha1.ConditionBackupReady,
			metav1.ConditionUnknown,
			backupv1alpha1.ReasonNoRunYet,
			"no backup run has occurred yet",
		)
	}
	if meta.FindStatusCondition(policy.Status.Conditions, backupv1alpha1.ConditionRestoreVerified) == nil {
		r.setCondition(
			policy,
			backupv1alpha1.ConditionRestoreVerified,
			metav1.ConditionUnknown,
			backupv1alpha1.ReasonNoRunYet,
			"no verification run has occurred yet",
		)
	}
}

func (r *BackupPolicyReconciler) emitEvent(
	policy *backupv1alpha1.BackupPolicy,
	eventType, reason, message string,
) {
	if r.Recorder != nil {
		r.Recorder.Eventf(policy, nil, eventType, reason, reason, "%s", message)
	}
}

func boolToConditionStatus(b bool) metav1.ConditionStatus {
	if b {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

func getJobCompletionStatus(job *batchv1.Job) (complete bool, success bool, deadlineExceeded bool) {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			return true, true, false
		}
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			isDeadline := c.Reason == "DeadlineExceeded"
			return true, false, isDeadline
		}
	}
	return false, false, false
}

func isValidBackupFailureReason(reason string) bool {
	switch reason {
	case backupv1alpha1.ReasonSourceUnreachable,
		backupv1alpha1.ReasonStorageUnreachable,
		backupv1alpha1.ReasonBackupJobFailed:
		return true
	default:
		return false
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackupPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorder("backupvouch-controller")
	}
	if r.DefaultRunnerImage == "" {
		r.DefaultRunnerImage = jobs.DefaultRunnerImage
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&backupv1alpha1.BackupPolicy{}).
		Owns(&batchv1.Job{}).
		Named("backuppolicy").
		Complete(r)
}
