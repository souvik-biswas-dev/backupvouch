package v1alpha1

// Condition types, see docs/API.md.
const (
	ConditionSpecValid       = "SpecValid"
	ConditionBackupReady     = "BackupReady"
	ConditionRestoreVerified = "RestoreVerified"
)

// Condition reasons, see docs/API.md and docs/ARCHITECTURE.md.
const (
	// SpecValid=False
	ReasonInvalidSchedule       = "InvalidSchedule"
	ReasonInvalidTimeZone       = "InvalidTimeZone"
	ReasonRunnerImageNotAllowed = "RunnerImageNotAllowed"
	ReasonSpecValid             = "SpecValid"

	// BackupReady=False
	ReasonSourceUnreachable = "SourceUnreachable"
	ReasonBackupJobFailed   = "BackupJobFailed"
	ReasonBackupTimeout     = "BackupTimeout"

	// BackupReady=False or RestoreVerified=False
	ReasonStorageUnreachable  = "StorageUnreachable"
	ReasonSecretMissing       = "SecretMissing"
	ReasonInvalidRunnerResult = "InvalidRunnerResult"

	// RestoreVerified=False
	ReasonChecksumMismatch    = "ChecksumMismatch"
	ReasonRestoreFailed       = "RestoreFailed"
	ReasonCheckFailed         = "CheckFailed"
	ReasonVerificationTimeout = "VerificationTimeout"

	// RestoreVerified=Unknown
	ReasonVerificationDisabled = "VerificationDisabled"

	// True conditions
	ReasonBackupSucceeded    = "BackupSucceeded"
	ReasonVerificationPassed = "VerificationPassed"
	ReasonNoRunYet           = "NoRunYet"
)

const (
	// AnnotationReverify triggers a manual re-verification of the newest backup.
	// Its value is a Unix timestamp in seconds.
	AnnotationReverify = "backupvouch.dev/reverify"

	// FinalizerName is set on every BackupPolicy to clean up Jobs on delete.
	FinalizerName = "backup.backupvouch.dev/finalizer"

	// Labels set on Jobs and Pods, see docs/ARCHITECTURE.md.
	LabelPolicy    = "backupvouch.dev/policy"
	LabelRunID     = "backupvouch.dev/run-id"
	LabelRole      = "backupvouch.dev/role"
	LabelManagedBy = "app.kubernetes.io/managed-by"

	// Values of LabelRole.
	RoleBackup = "backup"
	RoleVerify = "verify"
	RolePrune  = "prune"

	// ManagedByValue is the value of LabelManagedBy.
	ManagedByValue = "backupvouch"
)
