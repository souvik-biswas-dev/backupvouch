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
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
	"github.com/souvik-biswas-dev/backupvouch/internal/checks"
	"github.com/souvik-biswas-dev/backupvouch/internal/jobs"
)

// BackupTerminationMessage represents the JSON payload from a successful or failed backup.
type BackupTerminationMessage struct {
	ObjectKey string `json:"objectKey,omitempty"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
	Error     *struct {
		Reason  string `json:"reason"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// VerifyTerminationMessage represents the JSON payload from a verify Job container.
type VerifyTerminationMessage struct {
	Result                 backupv1alpha1.VerificationResultType `json:"result"`
	Reason                 string                                `json:"reason,omitempty"`
	RestoreDurationSeconds float64                               `json:"restoreDurationSeconds,omitempty"`
	Checks                 []checks.CheckResult                  `json:"checks,omitempty"`
	ChecksTruncated        bool                                  `json:"checksTruncated,omitempty"`
}

// JobPodInspection holds observations from the Pods created by a Job.
type JobPodInspection struct {
	SecretMissing      bool
	TerminationMessage string
	OOMKilled          bool
	ExitCode           int32
	ExitReason         string
}

// InspectJobPods queries Pods for a Job and checks for missing secrets or termination messages.
func InspectJobPods(
	ctx context.Context,
	c client.Reader,
	namespace, jobName string,
	now time.Time,
) (JobPodInspection, error) {
	var podList corev1.PodList
	opts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{"batch.kubernetes.io/job-name": jobName},
	}

	if err := c.List(ctx, &podList, opts...); err != nil {
		return JobPodInspection{}, fmt.Errorf("failed to list pods for job %s: %w", jobName, err)
	}

	inspection := JobPodInspection{}
	if len(podList.Items) == 0 {
		return inspection, nil
	}

	// Examine newest pod first
	for i := range podList.Items {
		pod := &podList.Items[i]

		// Check for SecretMissing: CreateContainerConfigError for >= 60 seconds (docs/ARCHITECTURE.md line 130)
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CreateContainerConfigError" {
				if now.Sub(pod.CreationTimestamp.Time) >= 60*time.Second {
					inspection.SecretMissing = true
				}
			}

			if cs.Name == jobs.RunnerContainerName || len(pod.Status.ContainerStatuses) == 1 {
				if cs.State.Terminated != nil {
					term := cs.State.Terminated
					inspection.TerminationMessage = term.Message
					inspection.ExitCode = term.ExitCode
					inspection.ExitReason = term.Reason
					if term.Reason == "OOMKilled" {
						inspection.OOMKilled = true
					}
				}
			}
		}
	}

	return inspection, nil
}

// ParseBackupResult parses the termination message of a backup runner container.
func ParseBackupResult(rawMsg string) (*BackupTerminationMessage, error) {
	if rawMsg == "" {
		return nil, fmt.Errorf("empty termination message")
	}
	var res BackupTerminationMessage
	if err := json.Unmarshal([]byte(rawMsg), &res); err != nil {
		return nil, fmt.Errorf("invalid json in termination message: %w", err)
	}
	return &res, nil
}

// ParseVerifyResult parses the termination message of a verify runner container.
func ParseVerifyResult(rawMsg string) (*VerifyTerminationMessage, error) {
	if rawMsg == "" {
		return nil, fmt.Errorf("empty termination message")
	}
	var res VerifyTerminationMessage
	if err := json.Unmarshal([]byte(rawMsg), &res); err != nil {
		return nil, fmt.Errorf("invalid json in termination message: %w", err)
	}
	return &res, nil
}
