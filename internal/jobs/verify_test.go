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

	corev1 "k8s.io/api/core/v1"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
)

func TestVerifyJobName(t *testing.T) {
	name := VerifyJobName("app-db", 1761271200)
	expected := "app-db-verify-1761271200"
	if name != expected {
		t.Errorf("VerifyJobName = %q, expected %q", name, expected)
	}
	if len(name) > 63 {
		t.Errorf("VerifyJobName length %d exceeds 63", len(name))
	}
}

func TestBuildVerifyJob(t *testing.T) {
	policy := samplePolicy()
	policy.Spec.Verify = backupv1alpha1.VerifySpec{
		TimeoutSeconds: 600,
		Checks: []backupv1alpha1.Check{
			{Name: "users-count", SQL: "SELECT count(*) FROM users", Expect: "10"},
		},
	}

	runID := int64(1761271200)
	objectKey := "app/2026-10-24T02-00-00Z.dump"
	expectedSHA256 := "abc123def456"

	job, err := BuildVerifyJob(policy, runID, objectKey, expectedSHA256)
	if err != nil {
		t.Fatalf("unexpected error building verify job: %v", err)
	}

	// 1. Name and Namespace
	expectedName := "app-db-verify-1761271200"
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
		backupv1alpha1.LabelRole:      backupv1alpha1.RoleVerify,
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
	if owner.Kind != "BackupPolicy" || owner.Name != "app-db" {
		t.Errorf("unexpected owner reference: %+v", owner)
	}

	// 4. Job spec
	if job.Spec.BackoffLimit == nil || *job.Spec.BackoffLimit != 1 {
		t.Errorf("backoffLimit = %v, expected 1", job.Spec.BackoffLimit)
	}
	if job.Spec.ActiveDeadlineSeconds == nil || *job.Spec.ActiveDeadlineSeconds != 600 {
		t.Errorf("activeDeadlineSeconds = %v, expected 600", job.Spec.ActiveDeadlineSeconds)
	}
	if job.Spec.Template.Spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Errorf("restartPolicy = %v, expected Never", job.Spec.Template.Spec.RestartPolicy)
	}

	// 5. Scratch and Tmp Volumes
	volumeMap := make(map[string]corev1.Volume)
	for _, v := range job.Spec.Template.Spec.Volumes {
		volumeMap[v.Name] = v
	}
	scratchVol, hasScratch := volumeMap[ScratchVolumeName]
	if !hasScratch {
		t.Fatalf("missing scratch volume in pod spec")
	}
	if scratchVol.EmptyDir == nil || scratchVol.EmptyDir.SizeLimit.String() != "5Gi" {
		t.Errorf("scratch sizeLimit = %v, expected 5Gi", scratchVol.EmptyDir.SizeLimit)
	}

	// 6. Container
	if len(job.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(job.Spec.Template.Spec.Containers))
	}
	container := job.Spec.Template.Spec.Containers[0]
	if len(container.Args) != 1 || container.Args[0] != "verify" {
		t.Errorf("container.Args = %v, expected ['verify']", container.Args)
	}

	mountMap := make(map[string]corev1.VolumeMount)
	for _, m := range container.VolumeMounts {
		mountMap[m.Name] = m
	}
	if mountMap[ScratchVolumeName].MountPath != "/scratch" {
		t.Errorf("scratch mount path = %q, expected /scratch", mountMap[ScratchVolumeName].MountPath)
	}
	if mountMap[TmpVolumeName].MountPath != "/tmp" {
		t.Errorf("tmp mount path = %q, expected /tmp", mountMap[TmpVolumeName].MountPath)
	}

	// 7. Verify NO database credentials are in environment
	envMap := make(map[string]corev1.EnvVar)
	for _, env := range container.Env {
		envMap[env.Name] = env
	}

	if _, hasPGPass := envMap["PGPASSWORD"]; hasPGPass {
		t.Errorf("security violation: PGPASSWORD must NOT be present in verify Job")
	}
	if _, hasPGUser := envMap["PGUSER"]; hasPGUser {
		t.Errorf("security violation: PGUSER must NOT be present in verify Job")
	}

	if envMap["OBJECT_KEY"].Value != objectKey {
		t.Errorf("OBJECT_KEY = %q, expected %q", envMap["OBJECT_KEY"].Value, objectKey)
	}
	if envMap["EXPECTED_SHA256"].Value != expectedSHA256 {
		t.Errorf("EXPECTED_SHA256 = %q, expected %q", envMap["EXPECTED_SHA256"].Value, expectedSHA256)
	}
	if envMap["DATABASE_NAME"].Value != "app" {
		t.Errorf("DATABASE_NAME = %q, expected 'app'", envMap["DATABASE_NAME"].Value)
	}
	if envMap["SCRATCH_DIR"].Value != "/scratch" {
		t.Errorf("SCRATCH_DIR = %q, expected /scratch", envMap["SCRATCH_DIR"].Value)
	}
	if !strings.Contains(envMap["CHECKS_JSON"].Value, "users-count") {
		t.Errorf("CHECKS_JSON = %q does not contain checks", envMap["CHECKS_JSON"].Value)
	}
}

func TestBuildVerifyJobNameTooLong(t *testing.T) {
	policy := samplePolicy()
	policy.Name = strings.Repeat("x", 50)
	_, err := BuildVerifyJob(policy, 1761271200, "dump.dump", "hash")
	if err == nil {
		t.Errorf("expected error when verify job name exceeds 63 characters")
	}
}
