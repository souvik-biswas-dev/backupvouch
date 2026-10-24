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

const (
	// Environment variable names for storage configuration
	EnvStorageEndpoint  = "STORAGE_ENDPOINT"
	EnvStorageBucket    = "STORAGE_BUCKET"
	EnvStoragePrefix    = "STORAGE_PREFIX"
	EnvStorageUseTLS    = "STORAGE_USE_TLS"
	EnvStorageAccessKey = "STORAGE_ACCESS_KEY"
	EnvStorageSecretKey = "STORAGE_SECRET_KEY"
	EnvTmpDir           = "TMPDIR"

	// Secret keys in Kubernetes secrets
	SecretKeyAccessKey = "accessKey"
	SecretKeySecretKey = "secretKey"

	// Kubernetes API types
	KindBackupPolicy  = "BackupPolicy"
	KindJob           = "Job"
	APIVersionBatchV1 = "batch/v1"

	// Container capability and termination settings
	CapabilityAll      = "ALL"
	TerminationLogPath = "/dev/termination-log"
)
