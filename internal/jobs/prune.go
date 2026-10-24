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

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
)

// PruneJobName generates the deterministic Job name: <policy>-prune-<runID>.
func PruneJobName(policyName string, runID int64) string {
	return fmt.Sprintf("%s-prune-%d", policyName, runID)
}

// BuildPruneJob builds a batch/v1 Job that executes the prune subcommand.
// The Job is fully hardened per docs/ARCHITECTURE.md and docs/DECISIONS.md.
func BuildPruneJob(
	policy *backupv1alpha1.BackupPolicy,
	runID int64,
	protectedKey string,
) (*batchv1.Job, error) {
	if policy == nil {
		return nil, fmt.Errorf("policy must not be nil")
	}

	jobName := PruneJobName(policy.Name, runID)
	runIDStr := strconv.FormatInt(runID, 10)

	labels := map[string]string{
		backupv1alpha1.LabelPolicy:    policy.Name,
		backupv1alpha1.LabelRunID:     runIDStr,
		backupv1alpha1.LabelRole:      backupv1alpha1.RolePrune,
		backupv1alpha1.LabelManagedBy: backupv1alpha1.ManagedByValue,
	}

	image := DefaultRunnerImage
	if policy.Spec.RunnerImage != "" {
		image = policy.Spec.RunnerImage
	}

	keepLast := policy.Spec.Retention.KeepLast
	if keepLast <= 0 {
		keepLast = 7
	}

	backoffLimit := int32(0)
	timeoutSeconds := int64(300)
	nonRoot := true
	readOnlyRoot := true
	allowPrivEscalation := false
	automountSAToken := false
	uid := RunnerUID

	useTLSStr := strconv.FormatBool(policy.Spec.Storage.UseTLS)

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
			Value: useTLSStr,
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
			Name:  "KEEP_LAST",
			Value: strconv.FormatInt(int64(keepLast), 10),
		},
		{
			Name:  "PROTECTED_KEY",
			Value: protectedKey,
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
							Args:  []string{"prune"},
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
									Name:      TmpVolumeName,
									MountPath: TmpMountPath,
								},
							},
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
