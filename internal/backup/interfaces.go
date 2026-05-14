package backup

import "context"

type BackupExecutor interface {
	Backup(ctx context.Context, opts BackupOptions, callback ProgressCallback) (*BackupResult, error)
}

type RestoreExecutor interface {
	Restore(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error)
}

type BackupManager interface {
	ListBackups(ctx context.Context, opts ...BackupOptions) ([]BackupInfo, error)
	DeleteBackup(ctx context.Context, identifier string, opts ...BackupOptions) error
	GetBackupInfo(ctx context.Context, backupID string, opts ...BackupOptions) (map[string]string, error)
	DeleteAllBackups(ctx context.Context, opts ...BackupOptions) error
}

type BackupRegistry interface {
	RegisterBackup(ctx context.Context, backupPath string) error
	UnregisterBackup(ctx context.Context, backupID string) error
}

type BackupValidator interface {
	ValidateBackup(ctx context.Context, backupID string, opts ...BackupOptions) error
	VerifyBackupStatus(ctx context.Context) error
	DeleteInvalidBackups(ctx context.Context, opts ...BackupOptions) error
}

type DatabaseBackup interface {
	BackupExecutor
	RestoreExecutor
	BackupManager
	BackupRegistry
	BackupValidator

	Close() error
}
