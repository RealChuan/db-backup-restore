package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// BaseBackup 提供公共字段和默认方法实现
type BaseBackup struct {
	config *DBConfig
}

// NewBaseBackup 创建 BaseBackup 实例
func NewBaseBackup(config *DBConfig) *BaseBackup {
	return &BaseBackup{config: config}
}

// GetConfig 获取数据库配置
func (b *BaseBackup) GetConfig() *DBConfig {
	return b.config
}

// RegisterBackup 默认实现：大多数数据库不支持注册备份到目录库
func (b *BaseBackup) RegisterBackup(ctx context.Context, _ string) error {
	return NewNotSupportedError(ctx, "RegisterBackup", b.config.Type)
}

// UnregisterBackup 默认实现：大多数数据库不支持取消注册备份
func (b *BaseBackup) UnregisterBackup(ctx context.Context, _ string) error {
	return NewNotSupportedError(ctx, "UnregisterBackup", b.config.Type)
}

// VerifyBackupStatus 默认实现：大多数数据库不支持检查备份状态
func (b *BaseBackup) VerifyBackupStatus(ctx context.Context) error {
	return NewNotSupportedError(ctx, "VerifyBackupStatus", b.config.Type)
}

// DeleteInvalidBackups 默认实现：大多数数据库不支持删除无效备份记录
func (b *BaseBackup) DeleteInvalidBackups(ctx context.Context, _ ...BackupOptions) error {
	return NewNotSupportedError(ctx, "DeleteInvalidBackups", b.config.Type)
}

// ValidateBackup 默认实现：大多数数据库不支持完整验证备份文件完整性
func (b *BaseBackup) ValidateBackup(ctx context.Context, _ string, _ ...BackupOptions) error {
	return NewNotSupportedError(ctx, "ValidateBackup", b.config.Type)
}

// ListDatabases 默认实现：大多数数据库不支持列出所有数据库（如 Oracle 基于实例架构）。
func (b *BaseBackup) ListDatabases(ctx context.Context) ([]string, error) {
	return nil, NewNotSupportedError(ctx, "ListDatabases", b.config.Type)
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

// validateRestoreIdentifier 校验还原标识符并返回备份是否为目录。
// 校验：标识符非空、文件/目录存在、备份类型与实际文件类型匹配（空类型默认 logical）。
// 供基于文件系统的驱动（MySQL/PostgreSQL）复用，避免校验逻辑重复导致行为漂移。
func (b *BaseBackup) validateRestoreIdentifier(identifier string, backupType BackupType) (bool, error) {
	if identifier == "" {
		return false, errors.New("必须通过 --backup-identifier 参数指定备份文件或目录路径")
	}
	info, err := os.Stat(identifier)
	if err != nil {
		return false, fmt.Errorf("备份文件/目录不可访问 %s: %w", identifier, err)
	}
	isDir := info.IsDir()
	expectedLogical := backupType == BackupTypeLogical || backupType == ""
	expectedPhysical := backupType == BackupTypePhysical
	if expectedLogical && isDir {
		return false, fmt.Errorf("备份类型不匹配：指定为逻辑备份，但提供的是目录: %s", identifier)
	}
	if expectedPhysical && !isDir {
		return false, fmt.Errorf("备份类型不匹配：指定为物理备份，但提供的是文件: %s", identifier)
	}
	return isDir, nil
}

// getBackupDir 从备份选项中获取备份目录路径
// 如果未指定目标路径，则返回空字符串
func (b *BaseBackup) getBackupDir(options []BackupOptions) string {
	if len(options) > 0 && options[0].TargetPath != "" {
		return options[0].TargetPath
	}
	return ""
}

// EnableArchiveLogMode 默认实现：大多数数据库不支持归档模式管理（仅 Oracle 和达梦支持）
func (b *BaseBackup) EnableArchiveLogMode(ctx context.Context, _ string) error {
	return NewNotSupportedError(ctx, "EnableArchiveLogMode", b.config.Type)
}

// DisableArchiveLogMode 默认实现：大多数数据库不支持归档模式管理（仅 Oracle 和达梦支持）
func (b *BaseBackup) DisableArchiveLogMode(ctx context.Context) error {
	return NewNotSupportedError(ctx, "DisableArchiveLogMode", b.config.Type)
}

// Close 默认实现：无需释放资源
func (b *BaseBackup) Close() error {
	return nil
}

// backupFilenameRegex 匹配标准备份文件名格式：数据库名_日期_时间.sql
var backupFilenameRegex = regexp.MustCompile(`^(.+)_(\d{8})_(\d{6})\.sql$`)

// ExtractDatabaseName 从备份文件名中提取数据库名
func ExtractDatabaseName(backupFile string) string {
	baseName := filepath.Base(backupFile)
	if matches := backupFilenameRegex.FindStringSubmatch(baseName); len(matches) > 1 {
		return matches[1]
	}
	return filepath.Base(backupFile)
}

// GenerateBackupFilename 生成标准格式的备份文件名
func GenerateBackupFilename(name string, extension string) string {
	timestamp := time.Now().Format("20060102_150405")
	return fmt.Sprintf("%s_%s.%s", name, timestamp, extension)
}
