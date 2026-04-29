package backup

import "context"

type DatabaseBackup interface {
	Backup(ctx context.Context, opts BackupOptions, callback ProgressCallback) (*BackupResult, error)
	Restore(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error)
	ListBackups(ctx context.Context, opts ...BackupOptions) ([]BackupInfo, error)
	DeleteBackup(ctx context.Context, identifier string, opts ...BackupOptions) error
	GetBackupInfo(ctx context.Context, backupID string, opts ...BackupOptions) (map[string]string, error)
	DeleteAllBackups(ctx context.Context, opts ...BackupOptions) error

	ValidateBackup(ctx context.Context, backupID string, opts ...BackupOptions) error
	RegisterBackup(ctx context.Context, backupPath string) error
	UnregisterBackup(ctx context.Context, backupID string) error
	VerifyBackupStatus(ctx context.Context) error
	DeleteInvalidBackups(ctx context.Context, opts ...BackupOptions) error

	Close() error
}
