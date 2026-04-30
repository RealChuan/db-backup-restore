package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"db-backup-restore/pkg/utils"
)

// MySQLBackup 实现 DatabaseBackup 接口，针对 MySQL 数据库
type MySQLBackup struct {
	BaseBackup
	env           []string // 环境变量
	mysqlPath     string   // mysql 命令路径
	mysqldumpPath string   // mysqldump 命令路径
}

// NewMySQLBackup 创建 MySQL 备份实例
func NewMySQLBackup(config *DBConfig) (*MySQLBackup, error) {
	if config.Type != "mysql" {
		return nil, errors.New("config.Type 必须是 mysql")
	}

	mysqlPath := "mysql"
	mysqldumpPath := "mysqldump"

	if val, ok := config.Extra["MYSQL_BIN_PATH"]; ok && val != "" {
		mysqlPath = utils.AddExeExt(filepath.Join(val, "mysql"))
		mysqldumpPath = utils.AddExeExt(filepath.Join(val, "mysqldump"))
	}

	return &MySQLBackup{
		BaseBackup:    BaseBackup{config: config},
		mysqlPath:     mysqlPath,
		mysqldumpPath: mysqldumpPath,
	}, nil
}

// Backup 执行 MySQL 备份（根据类型调用不同实现）
func (m *MySQLBackup) Backup(ctx context.Context, opts BackupOptions, callback ProgressCallback) (*BackupResult, error) {
	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}

	if opts.Mode == BackupModeIncremental || opts.Mode == BackupModeDifferential {
		utils.Infof("MySQL 不支持增量/差异备份模式，将使用全量备份")
	}

	backupDir := opts.TargetPath
	if backupDir == "" {
		result.Error = errors.New("必须通过 -target-path 参数指定备份路径")
		return result, result.Error
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		result.Error = err
		return result, err
	}

	databaseName := m.config.Database
	databases := m.parseDatabaseNames(databaseName)

	switch opts.Type {
	case BackupTypePhysical:
		if len(databases) > 0 {
			utils.Warnf("物理备份将备份整个 MySQL 实例，指定的数据库列表 [%s] 将被忽略", strings.Join(databases, ", "))
		}
		return m.backupPhysical(ctx, backupDir, callback)

	case BackupTypeLogical:
		if len(databases) == 0 {
			return m.backupAllDatabasesLogical(ctx, backupDir, callback)
		}
		if len(databases) == 1 {
			return m.backupSingleDatabaseLogical(ctx, backupDir, databases[0], callback)
		}
		return m.backupMultipleDatabasesLogical(ctx, backupDir, databases, callback)

	default:
		result.Error = errors.New("MySQL 仅支持 logical 和 physical 备份类型")
		return result, result.Error
	}
}

// Restore 执行 MySQL 还原（根据备份类型调用不同实现）
func (m *MySQLBackup) Restore(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error) {
	backupIdentifier := opts.BackupIdentifier
	if backupIdentifier == "" {
		return nil, errors.New("必须通过 --backup-identifier 参数指定备份文件或目录路径")
	}

	info, err := os.Stat(backupIdentifier)
	if err != nil {
		return nil, fmt.Errorf("备份文件/目录不存在: %s", backupIdentifier)
	}

	// 检查备份类型是否与实际数据匹配
	isDir := info.IsDir()
	expectedLogical := opts.BackupType == BackupTypeLogical || opts.BackupType == "" // 默认逻辑备份
	expectedPhysical := opts.BackupType == BackupTypePhysical

	if expectedLogical && isDir {
		return nil, fmt.Errorf("备份类型不匹配：指定为逻辑备份，但提供的是目录: %s", backupIdentifier)
	}
	if expectedPhysical && !isDir {
		return nil, fmt.Errorf("备份类型不匹配：指定为物理备份，但提供的是文件: %s", backupIdentifier)
	}

	// 根据实际数据类型调用对应实现
	if isDir {
		return m.restorePhysical(ctx, opts, callback)
	}

	return m.restoreLogical(ctx, opts, callback)
}

// ListBackups 列出所有备份（从文件系统）
func (m *MySQLBackup) ListBackups(ctx context.Context, opts ...BackupOptions) ([]BackupInfo, error) {
	backupDir := m.getBackupDir(opts)
	if backupDir == "" {
		return nil, errors.New("必须通过 opts.TargetPath 指定备份目录")
	}

	var backups []BackupInfo

	files, err := filepath.Glob(filepath.Join(backupDir, "*.sql*"))
	if err != nil {
		return nil, fmt.Errorf("列出逻辑备份失败: %w", err)
	}

	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}

		bi := BackupInfo{
			BackupID:       filepath.Base(file),
			CompletionTime: info.ModTime(),
			Size:           info.Size(),
			BackupPath:     file,
			BackupType:     "LOGICAL",
		}
		backups = append(backups, bi)
	}

	dirs, err := filepath.Glob(filepath.Join(backupDir, "*_physical"))
	if err != nil {
		return nil, fmt.Errorf("列出物理备份失败: %w", err)
	}

	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}

		bi := BackupInfo{
			BackupID:       filepath.Base(dir),
			CompletionTime: info.ModTime(),
			Size:           utils.GetDirSize(dir),
			BackupPath:     dir,
			BackupType:     "PHYSICAL",
		}
		backups = append(backups, bi)
	}

	return backups, nil
}

// DeleteBackup 删除指定备份
func (m *MySQLBackup) DeleteBackup(ctx context.Context, identifier string, opts ...BackupOptions) error {
	var backupPath string
	if filepath.IsAbs(identifier) {
		backupPath = identifier
	} else {
		backupDir := m.getBackupDir(opts)
		if backupDir == "" {
			return errors.New("必须通过 opts.TargetPath 指定备份目录或提供绝对路径")
		}
		backupPath = filepath.Join(backupDir, identifier)
	}

	info, err := os.Stat(backupPath)
	if err != nil {
		return fmt.Errorf("备份不存在: %w", err)
	}

	if info.IsDir() {
		return os.RemoveAll(backupPath)
	}

	return os.Remove(backupPath)
}

// GetBackupInfo 获取指定备份的详细信息
func (m *MySQLBackup) GetBackupInfo(ctx context.Context, backupID string, opts ...BackupOptions) (map[string]string, error) {
	if backupID == "" {
		return nil, errors.New("必须指定备份文件路径")
	}

	var backupPath string
	if filepath.IsAbs(backupID) {
		backupPath = backupID
	} else {
		backupDir := m.getBackupDir(opts)
		if backupDir == "" {
			return nil, errors.New("必须通过 opts.TargetPath 指定备份目录或提供绝对路径")
		}
		backupPath = filepath.Join(backupDir, backupID)
	}

	info, err := os.Stat(backupPath)
	if err != nil {
		return nil, fmt.Errorf("获取备份信息失败: %w", err)
	}

	result := make(map[string]string)
	result["path"] = backupPath
	result["size"] = strconv.FormatInt(info.Size(), 10)
	result["mod_time"] = info.ModTime().Format(time.RFC3339)

	if info.IsDir() {
		result["backup_type"] = "PHYSICAL"
		result["size"] = strconv.FormatInt(utils.GetDirSize(backupPath), 10)
	} else {
		result["backup_type"] = "LOGICAL"
	}

	return result, nil
}

// DeleteAllBackups 删除所有备份
func (m *MySQLBackup) DeleteAllBackups(ctx context.Context, opts ...BackupOptions) error {
	backupDir := m.getBackupDir(opts)
	if backupDir == "" {
		return errors.New("必须通过 opts.TargetPath 指定备份目录")
	}

	files, err := filepath.Glob(filepath.Join(backupDir, "*.sql*"))
	if err != nil {
		return fmt.Errorf("列出逻辑备份失败: %w", err)
	}

	for _, file := range files {
		if err := os.Remove(file); err != nil {
			utils.Warnf("删除逻辑备份失败: %v", err)
		}
	}

	dirs, err := filepath.Glob(filepath.Join(backupDir, "*_physical"))
	if err != nil {
		return fmt.Errorf("列出物理备份失败: %w", err)
	}

	for _, dir := range dirs {
		if err := os.RemoveAll(dir); err != nil {
			utils.Warnf("删除物理备份失败: %v", err)
		}
	}

	return nil
}

// init 自动注册 MySQL 驱动
func init() {
	RegisterDriver(DriverMetadata{
		Name:                 "mysql",
		Version:              "1.0.0",
		Description:          "MySQL 数据库备份驱动，支持 mysqldump 逻辑备份和文件级物理备份",
		SupportedActions:     []string{"backup", "restore", "list", "delete", "info", "delete-all"},
		SupportedBackupTypes: []BackupType{BackupTypeLogical, BackupTypePhysical},
	}, func(config *DBConfig) (DatabaseBackup, error) {
		return NewMySQLBackup(config)
	})
}
