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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// The field names, defaults and validation here are defined in docs/API.md.
// If this file needs to differ from that document, change the document in the
// same change and add a record to docs/DECISIONS.md.

// Engine is the database engine of the source.
// +kubebuilder:validation:Enum=postgres
type Engine string

const (
	// EnginePostgres is the only engine supported in v1.
	EnginePostgres Engine = "postgres"
)

// SecretReference names a Secret in the same namespace as the BackupPolicy.
// BackupVouch never reads the Secret itself; it only mounts it into Jobs.
type SecretReference struct {
	// name is the name of the Secret.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`
}

// SourceSpec describes the database to back up.
type SourceSpec struct {
	// engine is the database engine. Only postgres is supported in v1.
	// +optional
	// +kubebuilder:default=postgres
	Engine Engine `json:"engine,omitempty"`

	// host is the host name or service name of the database server.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Host string `json:"host"`

	// port is the TCP port of the database server.
	// +optional
	// +kubebuilder:default=5432
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`

	// database is the name of the database to dump.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Database string `json:"database"`

	// credentialsSecretRef names a Secret with the keys username and password.
	// Use a read-only database role.
	// +required
	CredentialsSecretRef SecretReference `json:"credentialsSecretRef"`
}

// StorageSpec describes the S3-compatible storage that holds the backups.
type StorageSpec struct {
	// endpoint is host:port of the S3-compatible storage.
	// +required
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=260
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9.-]+:[0-9]{1,5}$`
	Endpoint string `json:"endpoint"`

	// bucket is the bucket name.
	// +required
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9][a-z0-9.-]*[a-z0-9]$`
	Bucket string `json:"bucket"`

	// prefix is prepended to every object key, for example "app/".
	// +optional
	// +kubebuilder:validation:MaxLength=512
	Prefix string `json:"prefix,omitempty"`

	// useTLS makes the runner connect to the endpoint over TLS.
	// +optional
	// +kubebuilder:default=false
	UseTLS bool `json:"useTLS,omitempty"`

	// credentialsSecretRef names a Secret with the keys accessKey and secretKey.
	// +required
	CredentialsSecretRef SecretReference `json:"credentialsSecretRef"`
}

// RetentionSpec controls how many backups are kept.
type RetentionSpec struct {
	// keepLast is the number of newest backups to keep. The newest verified
	// backup is never deleted, even if that means keeping more.
	// +optional
	// +kubebuilder:default=7
	// +kubebuilder:validation:Minimum=1
	KeepLast int32 `json:"keepLast,omitempty"`
}

// Check is one SQL check run against the restored copy.
type Check struct {
	// name identifies the check. It must be a DNS-label style name.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`

	// sql is a single statement that returns exactly one row and one column.
	// It runs only against the restored copy, in a read-only transaction.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2000
	SQL string `json:"sql"`

	// expect is the expected text of the single value. Booleans are compared
	// as "true" or "false".
	// +optional
	// +kubebuilder:default="true"
	Expect string `json:"expect,omitempty"`
}

// VerifySpec controls restore verification.
type VerifySpec struct {
	// enabled turns verification on. If false, a run ends after the backup.
	// It is a pointer so that an explicit false survives omitempty and the
	// server-side default of true.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`

	// timeoutSeconds is the deadline of the verification Job.
	// +optional
	// +kubebuilder:default=900
	// +kubebuilder:validation:Minimum=1
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`

	// scratchSize is the size limit of the emptyDir that holds the downloaded
	// dump and the scratch Postgres data. It must exceed the dump plus the
	// restored database.
	// +optional
	// +kubebuilder:default="5Gi"
	ScratchSize *resource.Quantity `json:"scratchSize,omitempty"`

	// checks are SQL checks run against the restored copy.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=20
	Checks []Check `json:"checks,omitempty"`
}

// BackupPolicySpec defines the desired state of BackupPolicy.
type BackupPolicySpec struct {
	// schedule is a five-field cron expression. The controller validates it
	// fully and reports SpecValid=False if it is not valid.
	// +required
	// +kubebuilder:validation:MinLength=9
	// +kubebuilder:validation:MaxLength=256
	// +kubebuilder:validation:Pattern=`^\S+(\s+\S+){4}$`
	Schedule string `json:"schedule"`

	// timeZone is an IANA time zone name used to evaluate the schedule.
	// +optional
	// +kubebuilder:default=UTC
	// +kubebuilder:validation:MaxLength=64
	TimeZone string `json:"timeZone,omitempty"`

	// suspend pauses scheduling. Running Jobs finish.
	// +optional
	// +kubebuilder:default=false
	Suspend bool `json:"suspend,omitempty"`

	// source is the database to back up.
	// +required
	Source SourceSpec `json:"source"`

	// storage is where backups are stored.
	// +required
	Storage StorageSpec `json:"storage"`

	// retention controls how many backups are kept.
	// +optional
	// +kubebuilder:default={}
	Retention RetentionSpec `json:"retention,omitzero"`

	// backupTimeoutSeconds is the deadline of the backup Job.
	// +optional
	// +kubebuilder:default=1800
	// +kubebuilder:validation:Minimum=1
	BackupTimeoutSeconds int32 `json:"backupTimeoutSeconds,omitempty"`

	// verify controls restore verification.
	// +optional
	// +kubebuilder:default={}
	Verify VerifySpec `json:"verify,omitzero"`

	// runnerImage is the image that runs backup, verify and prune. It must
	// match the operator's allow-list of runner images. If empty, the
	// operator's default image is used. A pinned tag is recommended.
	// +optional
	// +kubebuilder:validation:MaxLength=512
	RunnerImage string `json:"runnerImage,omitempty"`

	// resources are applied to the runner containers.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitzero"`
}

// Phase is the current stage of a run.
// +kubebuilder:validation:Enum=Idle;BackingUp;Verifying;Pruning
type Phase string

const (
	PhaseIdle      Phase = "Idle"
	PhaseBackingUp Phase = "BackingUp"
	PhaseVerifying Phase = "Verifying"
	PhasePruning   Phase = "Pruning"
)

// BackupResultType is the outcome of a backup.
// +kubebuilder:validation:Enum=Succeeded;Failed
type BackupResultType string

const (
	BackupSucceeded BackupResultType = "Succeeded"
	BackupFailed    BackupResultType = "Failed"
)

// VerificationResultType is the outcome of a verification.
// +kubebuilder:validation:Enum=Passed;Failed
type VerificationResultType string

const (
	VerificationPassed VerificationResultType = "Passed"
	VerificationFailed VerificationResultType = "Failed"
)

// BackupStatus describes the last backup.
type BackupStatus struct {
	// time is when the backup finished.
	// +optional
	Time *metav1.Time `json:"time,omitempty"`

	// result is Succeeded or Failed.
	// +optional
	Result BackupResultType `json:"result,omitempty"`

	// objectKey is the key of the stored backup.
	// +optional
	ObjectKey string `json:"objectKey,omitempty"`

	// sizeBytes is the size of the stored backup.
	// +optional
	SizeBytes int64 `json:"sizeBytes,omitempty"`

	// sha256 is the hex SHA-256 of the stored backup.
	// +optional
	SHA256 string `json:"sha256,omitempty"`

	// message is a short reason on failure.
	// +optional
	Message string `json:"message,omitempty"`
}

// VerificationStatus describes the last verification.
type VerificationStatus struct {
	// time is when the verification finished.
	// +optional
	Time *metav1.Time `json:"time,omitempty"`

	// result is Passed or Failed.
	// +optional
	Result VerificationResultType `json:"result,omitempty"`

	// objectKey is the backup object that was verified.
	// +optional
	ObjectKey string `json:"objectKey,omitempty"`

	// restoreDurationSeconds is the measured restore time in whole seconds.
	// (CRDs avoid floating-point fields, so the runner's fractional value is
	// rounded by the controller.)
	// +optional
	RestoreDurationSeconds int64 `json:"restoreDurationSeconds,omitempty"`

	// failedChecks lists the names of checks that failed.
	// +optional
	// +kubebuilder:validation:MaxItems=20
	FailedChecks []string `json:"failedChecks,omitempty"`

	// message is a short reason on failure.
	// +optional
	Message string `json:"message,omitempty"`
}

// RunRecord is one entry of the run history.
type RunRecord struct {
	// runID is the scheduled time of the run as a Unix timestamp.
	// +required
	RunID int64 `json:"runID"`

	// time is when the run finished.
	// +optional
	Time *metav1.Time `json:"time,omitempty"`

	// objectKey is the backup object of the run.
	// +optional
	ObjectKey string `json:"objectKey,omitempty"`

	// backupResult is the outcome of the backup.
	// +optional
	BackupResult BackupResultType `json:"backupResult,omitempty"`

	// verificationResult is the outcome of the verification, empty if none ran.
	// +optional
	VerificationResult VerificationResultType `json:"verificationResult,omitempty"`
}

// BackupPolicyStatus defines the observed state of BackupPolicy.
type BackupPolicyStatus struct {
	// observedGeneration is the last spec generation reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// phase is the current stage: Idle, BackingUp, Verifying or Pruning.
	// +optional
	Phase Phase `json:"phase,omitempty"`

	// currentRunID is the scheduled time (Unix timestamp) of the run in
	// progress, absent when idle.
	// +optional
	CurrentRunID int64 `json:"currentRunID,omitempty"`

	// lastRunID is the runID of the most recent run that was started.
	// +optional
	LastRunID int64 `json:"lastRunID,omitempty"`

	// lastReverifyID is the last value of the backupvouch.pages.dev/reverify
	// annotation that was acted on.
	// +optional
	LastReverifyID int64 `json:"lastReverifyID,omitempty"`

	// lastBackup describes the last backup.
	// +optional
	LastBackup *BackupStatus `json:"lastBackup,omitempty"`

	// lastVerification describes the last verification.
	// +optional
	LastVerification *VerificationStatus `json:"lastVerification,omitempty"`

	// history holds the last 10 runs, newest first.
	// +optional
	// +kubebuilder:validation:MaxItems=10
	History []RunRecord `json:"history,omitempty"`

	// conditions are SpecValid, BackupReady and RestoreVerified, see docs/API.md.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=bp
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=".spec.schedule"
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Last Backup",type=date,JSONPath=".status.lastBackup.time"
// +kubebuilder:printcolumn:name="Verified",type=string,JSONPath=".status.conditions[?(@.type==\"RestoreVerified\")].status"
// +kubebuilder:printcolumn:name="Restore Time",type=integer,JSONPath=".status.lastVerification.restoreDurationSeconds"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"
// +kubebuilder:validation:XValidation:rule="self.metadata.name.size() <= 45",message="metadata.name must be at most 45 characters so Job names stay within 63 characters"

// BackupPolicy is the Schema for the backuppolicies API
type BackupPolicy struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of BackupPolicy
	// +required
	Spec BackupPolicySpec `json:"spec"`

	// status defines the observed state of BackupPolicy
	// +optional
	Status BackupPolicyStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// BackupPolicyList contains a list of BackupPolicy
type BackupPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []BackupPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &BackupPolicy{}, &BackupPolicyList{})
		return nil
	})
}
