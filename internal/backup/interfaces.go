package backup

import "context"

// ArchiveModeManager 归档模式管理接口（仅 Oracle 和达梦支持）
type ArchiveModeManager interface {
	EnableArchiveLogMode(ctx context.Context, archiveDest string) error
	DisableArchiveLogMode(ctx context.Context) error
}

// DatabaseBackup 完整的数据库备份管理接口
type DatabaseBackup interface {
	// Backup 执行备份
	Backup(ctx context.Context, opts BackupOptions, callback ProgressCallback) (*BackupResult, error)
	// Restore 执行还原
	Restore(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error)
	// ListBackups 列出所有备份
	ListBackups(ctx context.Context, opts ...BackupOptions) ([]BackupInfo, error)
	// DeleteBackup 删除指定备份
	DeleteBackup(ctx context.Context, identifier string, opts ...BackupOptions) error
	// GetBackupInfo 获取备份详细信息
	GetBackupInfo(ctx context.Context, backupID string, opts ...BackupOptions) (map[string]string, error)
	// ValidateBackup 验证备份
	ValidateBackup(ctx context.Context, backupID string, opts ...BackupOptions) error
	// RegisterBackup 注册备份
	RegisterBackup(ctx context.Context, backupPath string) error
	// UnregisterBackup 注销备份
	UnregisterBackup(ctx context.Context, backupID string) error
	// VerifyBackupStatus 验证备份状态
	VerifyBackupStatus(ctx context.Context) error
	// DeleteInvalidBackups 删除无效备份
	DeleteInvalidBackups(ctx context.Context, opts ...BackupOptions) error
	// DeleteAllBackups 删除所有备份
	DeleteAllBackups(ctx context.Context, opts ...BackupOptions) error
	// ListDatabases 列出所有数据库
	ListDatabases(ctx context.Context) ([]string, error)
	// Close 释放资源
	Close() error
}
