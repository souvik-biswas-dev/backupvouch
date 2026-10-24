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
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
)

func samplePolicy() *backupv1alpha1.BackupPolicy {
	return &backupv1alpha1.BackupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-db",
			Namespace: "default",
			UID:       types.UID("test-uid-1234"),
		},
		Spec: backupv1alpha1.BackupPolicySpec{
			Schedule: "0 2 * * *",
			TimeZone: "UTC",
			Source: backupv1alpha1.SourceSpec{
				Engine:   backupv1alpha1.EnginePostgres,
				Host:     "postgres.default.svc",
				Port:     5432,
				Database: "app",
				CredentialsSecretRef: backupv1alpha1.SecretReference{
					Name: "pg-creds",
				},
			},
			Storage: backupv1alpha1.StorageSpec{
				Endpoint: "minio.default.svc:9000",
				Bucket:   "backups",
				Prefix:   "app/",
				UseTLS:   false,
				CredentialsSecretRef: backupv1alpha1.SecretReference{
					Name: "minio-creds",
				},
			},
			BackupTimeoutSeconds: 1800,
		},
	}
}

func TestBackupJobName(t *testing.T) {
	name := BackupJobName("app-db", 1761271200)
	expected := "app-db-backup-1761271200"
	if name != expected {
		t.Errorf("BackupJobName = %q, expected %q", name, expected)
	}
	if len(name) > 63 {
		t.Errorf("BackupJobName length %d exceeds 63", len(name))
	}
}

func TestBackupObjectKey(t *testing.T) {
	schedTime := time.Date(2026, 10, 24, 2, 0, 0, 0, time.UTC)
	key := BackupObjectKey("app/", schedTime)
	expected := "app/2026-10-24T02-00-00Z.dump"
	if key != expected {
		t.Errorf("BackupObjectKey = %q, expected %q", key, expected)
	}
}

func TestBuildBackupJob(t *testing.T) {
	policy := samplePolicy()
	runID := int64(1761271200) // 2025-10-24 02:00:00 UTC

	job, err := BuildBackupJob(policy, runID)
	if err != nil {
		t.Fatalf("unexpected error building backup job: %v", err)
	}

	// 1. Job name and namespace
	expectedName := "app-db-backup-1761271200"
	if job.Name != expectedName {
		t.Errorf("job.Name = %q, expected %q", job.Name, expectedName)
	}
	if job.Namespace != "default" {
		t.Errorf("job.Namespace = %q, expected 'default'", job.Namespace)
	}

	// 2. Labels
	expectedLabels := map[string]string{
		backupv1alpha1.LabelPolicy:    "app-db",
		backupv1alpha1.LabelRunID:     "1761271200",
		backupv1alpha1.LabelRole:      backupv1alpha1.RoleBackup,
		backupv1alpha1.LabelManagedBy: backupv1alpha1.ManagedByValue,
	}
	for k, v := range expectedLabels {
		if job.Labels[k] != v {
			t.Errorf("Job label %q = %q, expected %q", k, job.Labels[k], v)
		}
		if job.Spec.Template.Labels[k] != v {
			t.Errorf("Pod label %q = %q, expected %q", k, job.Spec.Template.Labels[k], v)
		}
	}

	// 3. Owner Reference
	if len(job.OwnerReferences) != 1 {
		t.Fatalf("expected 1 owner reference, got %d", len(job.OwnerReferences))
	}
	owner := job.OwnerReferences[0]
	if owner.Kind != "BackupPolicy" || owner.Name != "app-db" || owner.UID != "test-uid-1234" {
		t.Errorf("unexpected owner reference: %+v", owner)
	}
	if owner.Controller == nil || !*owner.Controller {
		t.Errorf("owner.Controller should be true")
	}
	if owner.BlockOwnerDeletion == nil || !*owner.BlockOwnerDeletion {
		t.Errorf("owner.BlockOwnerDeletion should be true")
	}

	// 4. Job Spec (backoffLimit, activeDeadlineSeconds, restartPolicy)
	if job.Spec.BackoffLimit == nil || *job.Spec.BackoffLimit != 1 {
		t.Errorf("backoffLimit = %v, expected 1", job.Spec.BackoffLimit)
	}
	if job.Spec.ActiveDeadlineSeconds == nil || *job.Spec.ActiveDeadlineSeconds != 1800 {
		t.Errorf("activeDeadlineSeconds = %v, expected 1800", job.Spec.ActiveDeadlineSeconds)
	}
	if job.Spec.Template.Spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Errorf("restartPolicy = %v, expected Never", job.Spec.Template.Spec.RestartPolicy)
	}
	if job.Spec.Template.Spec.AutomountServiceAccountToken == nil || *job.Spec.Template.Spec.AutomountServiceAccountToken {
		t.Errorf("automountServiceAccountToken should be false")
	}

	// 5. Pod SecurityContext
	podSC := job.Spec.Template.Spec.SecurityContext
	if podSC == nil {
		t.Fatalf("Pod SecurityContext is nil")
	}
	if podSC.RunAsUser == nil || *podSC.RunAsUser != 70 {
		t.Errorf("pod runAsUser = %v, expected 70", podSC.RunAsUser)
	}
	if podSC.RunAsGroup == nil || *podSC.RunAsGroup != 70 {
		t.Errorf("pod runAsGroup = %v, expected 70", podSC.RunAsGroup)
	}
	if podSC.FSGroup == nil || *podSC.FSGroup != 70 {
		t.Errorf("pod fsGroup = %v, expected 70", podSC.FSGroup)
	}
	if podSC.RunAsNonRoot == nil || !*podSC.RunAsNonRoot {
		t.Errorf("pod runAsNonRoot should be true")
	}
	if podSC.SeccompProfile == nil || podSC.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Errorf("pod seccompProfile should be RuntimeDefault")
	}

	// 6. Container & SecurityContext
	if len(job.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(job.Spec.Template.Spec.Containers))
	}
	container := job.Spec.Template.Spec.Containers[0]
	if container.Name != RunnerContainerName {
		t.Errorf("container.Name = %q, expected %q", container.Name, RunnerContainerName)
	}
	if container.Image != DefaultRunnerImage {
		t.Errorf("container.Image = %q, expected %q", container.Image, DefaultRunnerImage)
	}
	if len(container.Args) != 1 || container.Args[0] != "backup" {
		t.Errorf("container.Args = %v, expected ['backup']", container.Args)
	}

	cSC := container.SecurityContext
	if cSC == nil {
		t.Fatalf("container SecurityContext is nil")
	}
	if cSC.AllowPrivilegeEscalation == nil || *cSC.AllowPrivilegeEscalation {
		t.Errorf("allowPrivilegeEscalation should be false")
	}
	if cSC.ReadOnlyRootFilesystem == nil || !*cSC.ReadOnlyRootFilesystem {
		t.Errorf("readOnlyRootFilesystem should be true")
	}
	if cSC.RunAsNonRoot == nil || !*cSC.RunAsNonRoot {
		t.Errorf("container runAsNonRoot should be true")
	}
	if cSC.RunAsUser == nil || *cSC.RunAsUser != 70 {
		t.Errorf("container runAsUser = %v, expected 70", cSC.RunAsUser)
	}
	if cSC.Capabilities == nil || len(cSC.Capabilities.Drop) != 1 || cSC.Capabilities.Drop[0] != "ALL" {
		t.Errorf("container capabilities.drop should be ['ALL']")
	}

	// 7. Volume and VolumeMount
	if len(job.Spec.Template.Spec.Volumes) != 1 || job.Spec.Template.Spec.Volumes[0].Name != TmpVolumeName {
		t.Errorf("missing tmp volume in pod spec")
	}
	if len(container.VolumeMounts) != 1 || container.VolumeMounts[0].MountPath != "/tmp" {
		t.Errorf("missing /tmp volume mount in container")
	}

	// 8. Termination message settings
	if container.TerminationMessagePath != "/dev/termination-log" {
		t.Errorf("terminationMessagePath = %q, expected /dev/termination-log", container.TerminationMessagePath)
	}
	if container.TerminationMessagePolicy != corev1.TerminationMessageReadFile {
		t.Errorf("terminationMessagePolicy = %v, expected File", container.TerminationMessagePolicy)
	}

	// 9. Environment variables
	envMap := make(map[string]corev1.EnvVar)
	for _, env := range container.Env {
		envMap[env.Name] = env
	}

	if envMap["PGHOST"].Value != "postgres.default.svc" {
		t.Errorf("PGHOST = %q", envMap["PGHOST"].Value)
	}
	if envMap["PGPORT"].Value != "5432" {
		t.Errorf("PGPORT = %q", envMap["PGPORT"].Value)
	}
	if envMap["PGDATABASE"].Value != "app" {
		t.Errorf("PGDATABASE = %q", envMap["PGDATABASE"].Value)
	}
	if envMap["STORAGE_ENDPOINT"].Value != "minio.default.svc:9000" {
		t.Errorf("STORAGE_ENDPOINT = %q", envMap["STORAGE_ENDPOINT"].Value)
	}
	if envMap["OBJECT_KEY"].Value != "app/2025-10-24T02-00-00Z.dump" {
		t.Errorf("OBJECT_KEY = %q", envMap["OBJECT_KEY"].Value)
	}
	if envMap["RUN_ID"].Value != "1761271200" {
		t.Errorf("RUN_ID = %q", envMap["RUN_ID"].Value)
	}
	if envMap["TMPDIR"].Value != "/tmp" {
		t.Errorf("TMPDIR = %q", envMap["TMPDIR"].Value)
	}

	// Secrets verification
	pgUserSecret := envMap["PGUSER"].ValueFrom.SecretKeyRef
	if pgUserSecret.Name != "pg-creds" || pgUserSecret.Key != "username" {
		t.Errorf("PGUSER secret ref invalid: %+v", pgUserSecret)
	}
	pgPassSecret := envMap["PGPASSWORD"].ValueFrom.SecretKeyRef
	if pgPassSecret.Name != "pg-creds" || pgPassSecret.Key != "password" {
		t.Errorf("PGPASSWORD secret ref invalid: %+v", pgPassSecret)
	}
	s3AccessSecret := envMap["STORAGE_ACCESS_KEY"].ValueFrom.SecretKeyRef
	if s3AccessSecret.Name != "minio-creds" || s3AccessSecret.Key != "accessKey" {
		t.Errorf("STORAGE_ACCESS_KEY secret ref invalid: %+v", s3AccessSecret)
	}
	s3SecretSecret := envMap["STORAGE_SECRET_KEY"].ValueFrom.SecretKeyRef
	if s3SecretSecret.Name != "minio-creds" || s3SecretSecret.Key != "secretKey" {
		t.Errorf("STORAGE_SECRET_KEY secret ref invalid: %+v", s3SecretSecret)
	}
}

func TestBuildBackupJobCustomRunnerAndResources(t *testing.T) {
	policy := samplePolicy()
	policy.Spec.RunnerImage = "my-registry.io/custom-runner:v1.2.3"
	policy.Spec.Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}

	job, err := BuildBackupJob(policy, 1761271200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	container := job.Spec.Template.Spec.Containers[0]
	if container.Image != "my-registry.io/custom-runner:v1.2.3" {
		t.Errorf("container.Image = %q, expected custom image", container.Image)
	}
	if container.Resources.Requests.Cpu().String() != "500m" {
		t.Errorf("cpu request = %v, expected 500m", container.Resources.Requests.Cpu())
	}
}

func TestBuildBackupJobNameTooLong(t *testing.T) {
	policy := samplePolicy()
	// 50 characters name + "-backup-" (8 chars) + 10 digits runID = 68 chars (> 63)
	policy.Name = strings.Repeat("a", 50)
	_, err := BuildBackupJob(policy, 1761271200)
	if err == nil {
		t.Errorf("expected error when job name exceeds 63 characters")
	}
}
