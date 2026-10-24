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

package jobs

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
)

const (
	// ScratchVolumeName is the name of the emptyDir scratch volume.
	ScratchVolumeName = "scratch"
)

// VerifyJobName generates the deterministic Job name: <policy>-verify-<runID>.
// Policy names are limited to 45 chars, so the resulting name stays <= 63 chars.
func VerifyJobName(policyName string, runID int64) string {
	return fmt.Sprintf("%s-verify-%d", policyName, runID)
}

// BuildVerifyJob builds a batch/v1 Job that executes the verify subcommand.
// Hardened with a throwaway scratch emptyDir, no database credentials, and non-root UID 70.
func BuildVerifyJob(policy *backupv1alpha1.BackupPolicy, runID int64, objectKey, expectedSHA256 string) (*batchv1.Job, error) {
	if policy == nil {
		return nil, fmt.Errorf("policy cannot be nil")
	}

	jobName := VerifyJobName(policy.Name, runID)
	if len(jobName) > 63 {
		return nil, fmt.Errorf("job name %q exceeds Kubernetes 63-character limit", jobName)
	}

	runIDStr := strconv.FormatInt(runID, 10)
	if objectKey == "" {
		scheduledTime := time.Unix(runID, 0).UTC()
		objectKey = BackupObjectKey(policy.Spec.Storage.Prefix, scheduledTime)
	}

	// Resource requirements: use policy.Spec.Resources if defined, else modest defaults.
	resources := policy.Spec.Resources
	if resources.Limits == nil && resources.Requests == nil {
		resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
		}
	}

	// Runner image: use spec.runnerImage if provided, else DefaultRunnerImage.
	image := policy.Spec.RunnerImage
	if image == "" {
		image = DefaultRunnerImage
	}

	// Verification timeout: use policy.Spec.Verify.TimeoutSeconds if non-zero.
	timeoutSeconds := int64(policy.Spec.Verify.TimeoutSeconds)
	if timeoutSeconds <= 0 {
		timeoutSeconds = int64(backupv1alpha1.DefaultVerifyTimeoutSeconds)
	}

	// Scratch size: use policy.Spec.Verify.ScratchSize if defined, else DefaultScratchSize (5Gi).
	scratchSize := policy.Spec.Verify.ScratchSize
	if scratchSize == nil {
		size := resource.MustParse(backupv1alpha1.DefaultScratchSize)
		scratchSize = &size
	}

	checksJSON, err := json.Marshal(policy.Spec.Verify.Checks)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal checks to JSON: %w", err)
	}

	backoffLimit := int32(1)
	uid := RunnerUID
	nonRoot := true
	allowPrivEscalation := false
	readOnlyRoot := true
	automountSAToken := false

	labels := map[string]string{
		backupv1alpha1.LabelPolicy:    policy.Name,
		backupv1alpha1.LabelRunID:     runIDStr,
		backupv1alpha1.LabelRole:      backupv1alpha1.RoleVerify,
		backupv1alpha1.LabelManagedBy: backupv1alpha1.ManagedByValue,
	}

	// Verification Job gets ONLY storage credentials, NEVER source database credentials
	env := []corev1.EnvVar{
		{
			Name:  "STORAGE_ENDPOINT",
			Value: policy.Spec.Storage.Endpoint,
		},
		{
			Name:  "STORAGE_BUCKET",
			Value: policy.Spec.Storage.Bucket,
		},
		{
			Name:  "STORAGE_PREFIX",
			Value: policy.Spec.Storage.Prefix,
		},
		{
			Name:  "STORAGE_USE_TLS",
			Value: strconv.FormatBool(policy.Spec.Storage.UseTLS),
		},
		{
			Name: "STORAGE_ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: policy.Spec.Storage.CredentialsSecretRef.Name,
					},
					Key: "accessKey",
				},
			},
		},
		{
			Name: "STORAGE_SECRET_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: policy.Spec.Storage.CredentialsSecretRef.Name,
					},
					Key: "secretKey",
				},
			},
		},
		{
			Name:  "OBJECT_KEY",
			Value: objectKey,
		},
		{
			Name:  "EXPECTED_SHA256",
			Value: expectedSHA256,
		},
		{
			Name:  "DATABASE_NAME",
			Value: policy.Spec.Source.Database,
		},
		{
			Name:  "CHECKS_JSON",
			Value: string(checksJSON),
		},
		{
			Name:  "RUN_ID",
			Value: runIDStr,
		},
		{
			Name:  "SCRATCH_DIR",
			Value: "/scratch",
		},
		{
			Name:  "TMPDIR",
			Value: TmpMountPath,
		},
	}

	isController := true
	blockOwnerDeletion := true
	ownerRefs := []metav1.OwnerReference{
		{
			APIVersion:         backupv1alpha1.GroupVersion.String(),
			Kind:               "BackupPolicy",
			Name:               policy.Name,
			UID:                policy.UID,
			Controller:         &isController,
			BlockOwnerDeletion: &blockOwnerDeletion,
		},
	}

	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            jobName,
			Namespace:       policy.Namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:          &backoffLimit,
			ActiveDeadlineSeconds: &timeoutSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					RestartPolicy:                corev1.RestartPolicyNever,
					AutomountServiceAccountToken: &automountSAToken,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &nonRoot,
						RunAsUser:    &uid,
						RunAsGroup:   &uid,
						FSGroup:      &uid,
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: ScratchVolumeName,
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{
									SizeLimit: scratchSize,
								},
							},
						},
						{
							Name: TmpVolumeName,
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  RunnerContainerName,
							Image: image,
							Args:  []string{"verify"},
							Env:   env,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &allowPrivEscalation,
								ReadOnlyRootFilesystem:   &readOnlyRoot,
								RunAsNonRoot:             &nonRoot,
								RunAsUser:                &uid,
								RunAsGroup:               &uid,
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      ScratchVolumeName,
									MountPath: "/scratch",
								},
								{
									Name:      TmpVolumeName,
									MountPath: TmpMountPath,
								},
							},
							Resources:                resources,
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: corev1.TerminationMessageReadFile,
						},
					},
				},
			},
		},
	}

	return job, nil
}
