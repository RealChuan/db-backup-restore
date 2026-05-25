package backup

import "context"

// Backuper 备份执行接口
type Backuper interface {
	Backup(ctx context.Context, opts BackupOptions, callback ProgressCallback) (*BackupResult, error)
}

// Restorer 还原执行接口
type Restorer interface {
	Restore(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error)
}

// BackupLister 备份列表查询接口
type BackupLister interface {
	ListBackups(ctx context.Context, opts ...BackupOptions) ([]BackupInfo, error)
}

// BackupDeleter 备份删除接口
type BackupDeleter interface {
	DeleteBackup(ctx context.Context, identifier string, opts ...BackupOptions) error
}

// BackupInfoGetter 备份信息查询接口
type BackupInfoGetter interface {
	GetBackupInfo(ctx context.Context, backupID string, opts ...BackupOptions) (map[string]string, error)
}

// BackupValidator 备份验证接口
type BackupValidator interface {
	ValidateBackup(ctx context.Context, backupID string, opts ...BackupOptions) error
}

// BackupRegistry 备份注册接口
type BackupRegistry interface {
	RegisterBackup(ctx context.Context, backupPath string) error
	UnregisterBackup(ctx context.Context, backupID string) error
}

// BackupStatusVerifier 备份状态验证接口
type BackupStatusVerifier interface {
	VerifyBackupStatus(ctx context.Context) error
	DeleteInvalidBackups(ctx context.Context, opts ...BackupOptions) error
}

// AllBackupDeleter 全量删除接口
type AllBackupDeleter interface {
	DeleteAllBackups(ctx context.Context, opts ...BackupOptions) error
}

// Closer 资源释放接口
type Closer interface {
	Close() error
}

// DatabaseBackup 完整的数据库备份管理接口（组合接口）
// 实现此接口的类型支持所有操作，但调用方应按需使用小接口
type DatabaseBackup interface {
	Backuper
	Restorer
	BackupLister
	BackupDeleter
	BackupInfoGetter
	BackupValidator
	BackupRegistry
	BackupStatusVerifier
	AllBackupDeleter
	Closer
}
