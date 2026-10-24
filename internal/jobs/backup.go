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
	// DefaultRunnerImage is the pinned runner image bundled with PostgreSQL 16 tools.
	DefaultRunnerImage = "ghcr.io/souvik-biswas-dev/backupvouch-runner:v0.1.0"

	// RunnerUID is UID 70 (postgres user in alpine postgres image).
	RunnerUID int64 = 70

	// RunnerContainerName is the container name inside the Job pod.
	RunnerContainerName = "runner"

	// TmpVolumeName is the name of the emptyDir /tmp volume.
	TmpVolumeName = "tmp"

	// TmpMountPath is the directory path for the temporary scratch/tmp volume.
	TmpMountPath = "/tmp"
)

// BackupJobName generates the deterministic Job name: <policy>-backup-<runID>.
// Policy names are limited to 45 chars, so the resulting name stays <= 63 chars.
func BackupJobName(policyName string, runID int64) string {
	return fmt.Sprintf("%s-backup-%d", policyName, runID)
}

// BackupObjectKey generates the S3 object key: <prefix><UTC-timestamp>.dump
// Example: "app/2026-10-24T02-00-00Z.dump".
func BackupObjectKey(prefix string, scheduledTime time.Time) string {
	formattedTime := scheduledTime.UTC().Format("2006-01-02T15-04-05Z")
	return fmt.Sprintf("%s%s.dump", prefix, formattedTime)
}

// BuildBackupJob builds a batch/v1 Job that executes the backup subcommand.
// The Job is fully hardened according to docs/ARCHITECTURE.md and docs/DECISIONS.md.
func BuildBackupJob(policy *backupv1alpha1.BackupPolicy, runID int64) (*batchv1.Job, error) {
	if policy == nil {
		return nil, fmt.Errorf("policy cannot be nil")
	}

	jobName := BackupJobName(policy.Name, runID)
	if len(jobName) > 63 {
		return nil, fmt.Errorf("job name %q exceeds Kubernetes 63-character limit", jobName)
	}

	runIDStr := strconv.FormatInt(runID, 10)
	scheduledTime := time.Unix(runID, 0).UTC()
	objectKey := BackupObjectKey(policy.Spec.Storage.Prefix, scheduledTime)

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

	// Deadline and timeouts: use policy.Spec.BackupTimeoutSeconds if non-zero.
	timeoutSeconds := int64(policy.Spec.BackupTimeoutSeconds)
	if timeoutSeconds <= 0 {
		timeoutSeconds = int64(backupv1alpha1.DefaultBackupTimeoutSeconds)
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
		backupv1alpha1.LabelRole:      backupv1alpha1.RoleBackup,
		backupv1alpha1.LabelManagedBy: backupv1alpha1.ManagedByValue,
	}

	port := policy.Spec.Source.Port
	if port <= 0 {
		port = backupv1alpha1.DefaultPostgresPort
	}

	env := []corev1.EnvVar{
		{
			Name:  "PGHOST",
			Value: policy.Spec.Source.Host,
		},
		{
			Name:  "PGPORT",
			Value: strconv.Itoa(int(port)),
		},
		{
			Name:  "PGDATABASE",
			Value: policy.Spec.Source.Database,
		},
		{
			Name: "PGUSER",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: policy.Spec.Source.CredentialsSecretRef.Name,
					},
					Key: "username",
				},
			},
		},
		{
			Name: "PGPASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: policy.Spec.Source.CredentialsSecretRef.Name,
					},
					Key: "password",
				},
			},
		},
		{
			Name:  EnvStorageEndpoint,
			Value: policy.Spec.Storage.Endpoint,
		},
		{
			Name:  EnvStorageBucket,
			Value: policy.Spec.Storage.Bucket,
		},
		{
			Name:  EnvStoragePrefix,
			Value: policy.Spec.Storage.Prefix,
		},
		{
			Name:  EnvStorageUseTLS,
			Value: strconv.FormatBool(policy.Spec.Storage.UseTLS),
		},
		{
			Name: EnvStorageAccessKey,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: policy.Spec.Storage.CredentialsSecretRef.Name,
					},
					Key: SecretKeyAccessKey,
				},
			},
		},
		{
			Name: EnvStorageSecretKey,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: policy.Spec.Storage.CredentialsSecretRef.Name,
					},
					Key: SecretKeySecretKey,
				},
			},
		},
		{
			Name:  "OBJECT_KEY",
			Value: objectKey,
		},
		{
			Name:  "RUN_ID",
			Value: runIDStr,
		},
		{
			Name:  EnvTmpDir,
			Value: TmpMountPath,
		},
	}

	isController := true
	blockOwnerDeletion := true
	ownerRefs := []metav1.OwnerReference{
		{
			APIVersion:         backupv1alpha1.GroupVersion.String(),
			Kind:               KindBackupPolicy,
			Name:               policy.Name,
			UID:                policy.UID,
			Controller:         &isController,
			BlockOwnerDeletion: &blockOwnerDeletion,
		},
	}

	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: APIVersionBatchV1,
			Kind:       KindJob,
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
							Args:  []string{"backup"},
							Env:   env,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &allowPrivEscalation,
								ReadOnlyRootFilesystem:   &readOnlyRoot,
								RunAsNonRoot:             &nonRoot,
								RunAsUser:                &uid,
								RunAsGroup:               &uid,
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{CapabilityAll},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      TmpVolumeName,
									MountPath: TmpMountPath,
								},
							},
							Resources:                resources,
							TerminationMessagePath:   TerminationLogPath,
							TerminationMessagePolicy: corev1.TerminationMessageReadFile,
						},
					},
				},
			},
		},
	}

	return job, nil
}
