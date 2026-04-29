package backup

import (
	"context"
	"strings"
)

// BaseBackup 提供公共字段和默认方法实现
type BaseBackup struct {
	config      *DBConfig
	initialized bool
}

// NewBaseBackup 创建 BaseBackup 实例
func NewBaseBackup(config *DBConfig) *BaseBackup {
	return &BaseBackup{config: config, initialized: false}
}

// GetConfig 获取数据库配置
func (b *BaseBackup) GetConfig() *DBConfig {
	return b.config
}

// IsInitialized 检查驱动是否已初始化
func (b *BaseBackup) IsInitialized() bool {
	return b.initialized
}

// RegisterBackup 默认实现：大多数数据库不支持注册备份到目录库
func (b *BaseBackup) RegisterBackup(ctx context.Context, backupPath string) error {
	return NewNotSupportedError("RegisterBackup", b.config.Type)
}

// UnregisterBackup 默认实现：大多数数据库不支持取消注册备份
func (b *BaseBackup) UnregisterBackup(ctx context.Context, backupID string) error {
	return NewNotSupportedError("UnregisterBackup", b.config.Type)
}

// VerifyBackupStatus 默认实现：大多数数据库不支持检查备份状态
func (b *BaseBackup) VerifyBackupStatus(ctx context.Context) error {
	return NewNotSupportedError("VerifyBackupStatus", b.config.Type)
}

// DeleteInvalidBackups 默认实现：大多数数据库不支持删除无效备份记录
func (b *BaseBackup) DeleteInvalidBackups(ctx context.Context, opts ...BackupOptions) error {
	return NewNotSupportedError("DeleteInvalidBackups", b.config.Type)
}

// ValidateBackup 默认实现：大多数数据库不支持完整验证备份文件完整性
func (b *BaseBackup) ValidateBackup(ctx context.Context, backupID string, opts ...BackupOptions) error {
	return NewNotSupportedError("ValidateBackup", b.config.Type)
}

// parseDatabaseNames 解析数据库名称（支持逗号分隔的多个数据库）
func (b *BaseBackup) parseDatabaseNames(databaseName string) []string {
	if databaseName == "" || databaseName == "all" {
		return nil
	}

	var names []string
	for _, name := range strings.Split(databaseName, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// getBackupDir 从备份选项中获取备份目录路径
// 如果未指定目标路径，则返回空字符串
func (b *BaseBackup) getBackupDir(options []BackupOptions) string {
	if len(options) > 0 && options[0].TargetPath != "" {
		return options[0].TargetPath
	}
	return ""
}

// Close 默认实现：无需释放资源
func (b *BaseBackup) Close() error {
	return nil
}
