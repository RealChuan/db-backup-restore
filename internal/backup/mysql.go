package backup

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/RealChuan/db-backup-restore/internal/logging"
	"github.com/RealChuan/db-backup-restore/pkg/fileutil"
)

// MySQLBackup 实现 DatabaseBackup 接口，针对 MySQL 数据库
type MySQLBackup struct {
	BaseBackup
	mysqlPath     string // mysql 命令路径
	mysqldumpPath string // mysqldump 命令路径
	fsManager     *FileSystemBackupManager
}

// NewMySQLBackup 创建 MySQL 备份实例
func NewMySQLBackup(config *DBConfig) (*MySQLBackup, error) {
	if config.Type != DBTypeMySQL {
		return nil, errors.New("config.Type 必须是 mysql")
	}

	mysqlPath := "mysql"         //nolint:goconst // 命令名，非数据库类型常量
	mysqldumpPath := "mysqldump" //nolint:goconst // 命令名

	if val := config.GetExtraTyped().MySQLBinPath(); val != "" {
		mysqlPath = fileutil.AddExeExt(filepath.Join(val, "mysql"))
		mysqldumpPath = fileutil.AddExeExt(filepath.Join(val, "mysqldump"))
	}

	return &MySQLBackup{
		BaseBackup:    BaseBackup{config: config},
		mysqlPath:     mysqlPath,
		mysqldumpPath: mysqldumpPath,
		fsManager:     NewFileSystemBackupManager(""),
	}, nil
}

// Backup 执行 MySQL 备份（根据类型调用不同实现）
func (m *MySQLBackup) Backup(ctx context.Context, opts BackupOptions, callback ProgressCallback) (*BackupResult, error) {
	if opts.Mode == BackupModeIncremental || opts.Mode == BackupModeDifferential {
		return nil, NewNotSupportedError(ctx, "backup", "mysql")
	}

	backupDir := opts.TargetPath
	if backupDir == "" {
		return nil, errors.New("必须通过 -target-path 参数指定备份路径")
	}
	if err := fileutil.EnsureDir(backupDir); err != nil {
		return nil, err
	}

	databaseName := m.config.Database
	databases := m.parseDatabaseNames(databaseName)

	switch opts.Type {
	case BackupTypePhysical:
		if len(databases) > 0 {
			logging.WarnCtx(ctx, "物理备份将备份整个实例，指定数据库列表将被忽略", "databases", strings.Join(databases, ", "))
		}
		return m.backupPhysical(ctx, backupDir, callback)

	case BackupTypeLogical:
		if len(databases) == 0 {
			return m.backupLogicalAll(ctx, backupDir, callback)
		}
		if len(databases) == 1 {
			return m.backupLogicalSingle(ctx, backupDir, databases[0], callback)
		}
		return m.backupLogicalMultiple(ctx, backupDir, databases, callback)

	default:
		return nil, errors.New("MySQL 仅支持 logical 和 physical 备份类型")
	}
}

// Restore 执行 MySQL 还原（根据备份类型调用不同实现）
func (m *MySQLBackup) Restore(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error) {
	isDir, err := m.validateRestoreIdentifier(opts.BackupIdentifier, opts.BackupType)
	if err != nil {
		return nil, err
	}

	// 根据实际数据类型调用对应实现
	if isDir {
		return m.restorePhysical(ctx, opts, callback)
	}

	return m.restoreLogical(ctx, opts, callback)
}

// ListBackups 列出所有备份（委托给 FileSystemBackupManager）
func (m *MySQLBackup) ListBackups(ctx context.Context, opts ...BackupOptions) ([]BackupInfo, error) {
	return m.fsManager.ListBackups(ctx, m.getBackupDir(opts))
}

// DeleteBackup 删除指定备份（委托给 FileSystemBackupManager）
func (m *MySQLBackup) DeleteBackup(ctx context.Context, identifier string, opts ...BackupOptions) error {
	return m.fsManager.DeleteBackup(ctx, identifier, m.getBackupDir(opts))
}

// GetBackupInfo 获取指定备份的详细信息（委托给 FileSystemBackupManager）
func (m *MySQLBackup) GetBackupInfo(ctx context.Context, backupID string, opts ...BackupOptions) (map[string]string, error) {
	return m.fsManager.GetBackupInfo(ctx, backupID, m.getBackupDir(opts))
}

// DeleteAllBackups 删除所有备份（委托给 FileSystemBackupManager）
func (m *MySQLBackup) DeleteAllBackups(ctx context.Context, opts ...BackupOptions) error {
	return m.fsManager.DeleteAllBackups(ctx, m.getBackupDir(opts))
}

// registerMySQLDriver 注册 MySQL 驱动
func registerMySQLDriver() error {
	return RegisterDriver(DriverMetadata{
		Name:                 DBTypeMySQL,
		Version:              versionXML,
		Description:          "MySQL 数据库备份驱动，支持 mysqldump 逻辑备份和文件级物理备份",
		SupportedActions:     []string{backupTypeXML, actionRestore, actionList, actionDelete, actionInfo, actionDeleteAll},
		SupportedBackupTypes: []BackupType{BackupTypeLogical, BackupTypePhysical},
	}, func(config *DBConfig) (DatabaseBackup, error) {
		return NewMySQLBackup(config)
	})
}
