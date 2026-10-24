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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
)

func TestBuildPruneJob(t *testing.T) {
	policy := &backupv1alpha1.BackupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "prod",
			UID:       types.UID("12345-67890"),
		},
		Spec: backupv1alpha1.BackupPolicySpec{
			Storage: backupv1alpha1.StorageSpec{
				Endpoint: "minio.prod.svc:9000",
				Bucket:   "backups",
				Prefix:   "test-app/",
				CredentialsSecretRef: backupv1alpha1.SecretReference{
					Name: "minio-creds",
				},
			},
			Retention: backupv1alpha1.RetentionSpec{
				KeepLast: 10,
			},
		},
	}

	runID := int64(1771900000)
	protectedKey := "test-app/verified-good.dump"

	t.Run("nil policy returns error", func(t *testing.T) {
		_, err := BuildPruneJob(nil, runID, protectedKey)
		if err == nil {
			t.Fatal("expected error for nil policy, got nil")
		}
	})

	t.Run("generates deterministic name and metadata", func(t *testing.T) {
		job, err := BuildPruneJob(policy, runID, protectedKey)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expectedName := "test-app-prune-1771900000"
		if job.Name != expectedName {
			t.Errorf("job.Name = %q, expected %q", job.Name, expectedName)
		}
		if job.Namespace != "prod" {
			t.Errorf("job.Namespace = %q, expected 'prod'", job.Namespace)
		}

		if job.Labels[backupv1alpha1.LabelRole] != backupv1alpha1.RolePrune {
			t.Errorf("role label = %q, expected %q", job.Labels[backupv1alpha1.LabelRole], backupv1alpha1.RolePrune)
		}
	})

	t.Run("hardened security and container settings", func(t *testing.T) {
		job, err := BuildPruneJob(policy, runID, protectedKey)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if *job.Spec.BackoffLimit != 0 {
			t.Errorf("backoffLimit = %d, expected 0", *job.Spec.BackoffLimit)
		}
		if *job.Spec.ActiveDeadlineSeconds != 300 {
			t.Errorf("activeDeadlineSeconds = %d, expected 300", *job.Spec.ActiveDeadlineSeconds)
		}

		podSpec := job.Spec.Template.Spec
		if *podSpec.AutomountServiceAccountToken {
			t.Errorf("automountServiceAccountToken should be false")
		}
		if *podSpec.SecurityContext.RunAsUser != RunnerUID {
			t.Errorf("runAsUser = %d, expected %d", *podSpec.SecurityContext.RunAsUser, RunnerUID)
		}

		container := podSpec.Containers[0]
		if len(container.Args) != 1 || container.Args[0] != "prune" {
			t.Errorf("container args = %v, expected [prune]", container.Args)
		}

		envMap := make(map[string]string)
		for _, e := range container.Env {
			envMap[e.Name] = e.Value
		}
		if envMap["KEEP_LAST"] != "10" {
			t.Errorf("KEEP_LAST = %q, expected '10'", envMap["KEEP_LAST"])
		}
		if envMap["PROTECTED_KEY"] != protectedKey {
			t.Errorf("PROTECTED_KEY = %q, expected %q", envMap["PROTECTED_KEY"], protectedKey)
		}
	})
}
