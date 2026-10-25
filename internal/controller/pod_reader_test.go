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
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
)

func TestParseBackupResult(t *testing.T) {
	t.Run("valid success message", func(t *testing.T) {
		raw := `{"objectKey":"app/2026-10-24T02-00-00Z.dump","sizeBytes":1234,"sha256":"abc"}`
		res, err := ParseBackupResult(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.ObjectKey != "app/2026-10-24T02-00-00Z.dump" || res.SizeBytes != 1234 {
			t.Errorf("unexpected parsed result: %+v", res)
		}
	})

	t.Run("valid error message", func(t *testing.T) {
		raw := `{"error":{"reason":"SourceUnreachable","message":"could not connect to host"}}`
		res, err := ParseBackupResult(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Error == nil || res.Error.Reason != "SourceUnreachable" {
			t.Errorf("unexpected error payload: %+v", res.Error)
		}
	})

	t.Run("invalid json returns error", func(t *testing.T) {
		_, err := ParseBackupResult("not json")
		if err == nil {
			t.Errorf("expected error for non-json, got nil")
		}
	})
}

func TestParseVerifyResult(t *testing.T) {
	t.Run("valid verify passed message", func(t *testing.T) {
		raw := `{"result":"Passed","reason":"","restoreDurationSeconds":12.3,"checks":[{"name":"u","passed":true}]}`
		res, err := ParseVerifyResult(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Result != backupv1alpha1.VerificationPassed || res.RestoreDurationSeconds != 12.3 {
			t.Errorf("unexpected verify result: %+v", res)
		}
	})
}

func TestInspectJobPods(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	now := time.Now()
	podCreatedOld := metav1.NewTime(now.Add(-70 * time.Second))

	podMissingSecret := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-job-pod-1",
			Namespace:         "default",
			CreationTimestamp: podCreatedOld,
			Labels: map[string]string{
				"batch.kubernetes.io/job-name": "test-job",
			},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "runner",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "CreateContainerConfigError",
						},
					},
				},
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(podMissingSecret).Build()

	inspection, err := InspectJobPods(context.Background(), client, "default", "test-job", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !inspection.SecretMissing {
		t.Errorf("expected SecretMissing=true when waiting in CreateContainerConfigError >= 60s")
	}
}

func TestInspectJobPodsUsesNewestPod(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	now := time.Now()
	newPod := func(name string, age time.Duration, message string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         "default",
				CreationTimestamp: metav1.NewTime(now.Add(-age)),
				Labels:            map[string]string{"batch.kubernetes.io/job-name": "retry-job"},
			},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{{
					Name:  "runner",
					State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1, Message: message}},
				}},
			},
		}
	}

	// The fake client lists by name. "a-new" sorts first although it is the newer Pod, so a reader that trusts
	// list order would return the old message.
	newest := newPod("a-new", 10*time.Second, `{"result":"Passed"}`)
	oldest := newPod("b-old", 5*time.Minute, `{"result":"Failed","reason":"RestoreFailed"}`)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(newest, oldest).Build()

	inspection, err := InspectJobPods(context.Background(), c, "default", "retry-job", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inspection.TerminationMessage != `{"result":"Passed"}` {
		t.Errorf("TerminationMessage = %q, want the newest Pod's message", inspection.TerminationMessage)
	}
}
