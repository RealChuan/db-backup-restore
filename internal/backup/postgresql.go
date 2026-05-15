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

type PostgreSQLBackup struct {
	BaseBackup
	psqlPath           string
	pgDumpPath         string
	pgDumpallPath      string
	pgBasebackupPath   string
	pgVerifyBackupPath string
	pgCtlPath          string
	env                []string
}

func NewPostgreSQLBackup(config *DBConfig) (*PostgreSQLBackup, error) {
	if config.Type != "postgresql" {
		return nil, errors.New("config.Type 必须是 postgresql")
	}

	psqlPath := "psql"
	pgDumpPath := "pg_dump"
	pgDumpallPath := "pg_dumpall"
	pgBasebackupPath := "pg_basebackup"
	pgVerifyBackupPath := "pg_verifybackup"
	pgCtlPath := "pg_ctl"

	if val, ok := config.Extra["PG_BIN_PATH"]; ok && val != "" {
		psqlPath = utils.AddExeExt(filepath.Join(val, "psql"))
		pgDumpPath = utils.AddExeExt(filepath.Join(val, "pg_dump"))
		pgDumpallPath = utils.AddExeExt(filepath.Join(val, "pg_dumpall"))
		pgBasebackupPath = utils.AddExeExt(filepath.Join(val, "pg_basebackup"))
		pgVerifyBackupPath = utils.AddExeExt(filepath.Join(val, "pg_verifybackup"))
		pgCtlPath = utils.AddExeExt(filepath.Join(val, "pg_ctl"))
	}

	env := []string{
		fmt.Sprintf("PGHOST=%s", config.Host),
		fmt.Sprintf("PGPORT=%d", config.Port),
		fmt.Sprintf("PGUSER=%s", config.User),
		fmt.Sprintf("PGPASSWORD=%s", config.Password),
		fmt.Sprintf("PGDATABASE=%s", config.Database),
	}

	if config.SSLMode != "" {
		env = append(env, fmt.Sprintf("PGSSLMODE=%s", config.SSLMode))
	}

	return &PostgreSQLBackup{
		BaseBackup:         BaseBackup{config: config},
		psqlPath:           psqlPath,
		pgDumpPath:         pgDumpPath,
		pgDumpallPath:      pgDumpallPath,
		pgBasebackupPath:   pgBasebackupPath,
		pgVerifyBackupPath: pgVerifyBackupPath,
		pgCtlPath:          pgCtlPath,
		env:                env,
	}, nil
}

func (p *PostgreSQLBackup) Backup(ctx context.Context, opts BackupOptions, callback ProgressCallback) (*BackupResult, error) {
	if opts.Mode == BackupModeIncremental || opts.Mode == BackupModeDifferential {
		utils.Infof("PostgreSQL 不支持增量/差异备份模式，将使用全量备份")
	}

	backupDir := opts.TargetPath
	if backupDir == "" {
		return nil, errors.New("必须通过 -target-path 参数指定备份路径")
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, err
	}

	databaseName := p.config.Database
	databases := p.parseDatabaseNames(databaseName)

	switch opts.Type {
	case BackupTypePhysical:
		if len(databases) > 0 {
			utils.Warnf("物理备份将备份整个 PostgreSQL 实例，指定的数据库列表 [%s] 将被忽略", strings.Join(databases, ", "))
		}
		return p.backupPhysical(ctx, backupDir, callback)

	case BackupTypeLogical:
		if len(databases) == 0 {
			return p.backupAllDatabasesLogical(ctx, backupDir, callback)
		}
		if len(databases) == 1 {
			return p.backupSingleDatabaseLogical(ctx, backupDir, databases[0], callback)
		}
		return p.backupMultipleDatabasesLogical(ctx, backupDir, databases, callback)

	default:
		return nil, errors.New("PostgreSQL 仅支持 logical 和 physical 备份类型")
	}
}

func (p *PostgreSQLBackup) Restore(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error) {
	backupIdentifier := opts.BackupIdentifier
	if backupIdentifier == "" {
		return nil, errors.New("必须通过 --backup-identifier 参数指定备份文件或目录路径")
	}

	info, err := os.Stat(backupIdentifier)
	if err != nil {
		return nil, fmt.Errorf("备份文件/目录不存在: %s", backupIdentifier)
	}

	isDir := info.IsDir()
	expectedLogical := opts.BackupType == BackupTypeLogical || opts.BackupType == ""
	expectedPhysical := opts.BackupType == BackupTypePhysical

	if expectedLogical && isDir {
		return nil, fmt.Errorf("备份类型不匹配：指定为逻辑备份，但提供的是目录: %s", backupIdentifier)
	}
	if expectedPhysical && !isDir {
		return nil, fmt.Errorf("备份类型不匹配：指定为物理备份，但提供的是文件: %s", backupIdentifier)
	}

	if isDir {
		return p.restorePhysical(ctx, opts, callback)
	}

	return p.restoreLogical(ctx, opts, callback)
}

func (p *PostgreSQLBackup) ListBackups(ctx context.Context, opts ...BackupOptions) ([]BackupInfo, error) {
	backupDir := p.getBackupDir(opts)
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

func (p *PostgreSQLBackup) DeleteBackup(ctx context.Context, identifier string, opts ...BackupOptions) error {
	var backupPath string
	if filepath.IsAbs(identifier) {
		cleanPath, err := sanitizeBackupPath(identifier)
		if err != nil {
			return fmt.Errorf("invalid backup path: %w", err)
		}
		backupPath = cleanPath
	} else {
		backupDir := p.getBackupDir(opts)
		if backupDir == "" {
			return errors.New("必须通过 opts.TargetPath 指定备份目录或提供绝对路径")
		}
		if strings.ContainsAny(identifier, `/\`) {
			return fmt.Errorf("backup identifier cannot contain path separators: %q", identifier)
		}
		backupPath = filepath.Join(backupDir, identifier)
		if err := mustBeUnderBackupDir(backupPath, backupDir); err != nil {
			return err
		}
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

func (p *PostgreSQLBackup) GetBackupInfo(ctx context.Context, backupID string, opts ...BackupOptions) (map[string]string, error) {
	if backupID == "" {
		return nil, errors.New("必须指定备份文件路径")
	}

	var backupPath string
	backupDir := p.getBackupDir(opts)

	if filepath.IsAbs(backupID) {
		cleanPath, err := sanitizeBackupPath(backupID)
		if err != nil {
			return nil, fmt.Errorf("invalid backup path: %w", err)
		}
		if err := mustBeUnderBackupDir(cleanPath, backupDir); err != nil {
			return nil, fmt.Errorf("backup path not in allowed directory: %w", err)
		}
		backupPath = cleanPath
	} else {
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

func (p *PostgreSQLBackup) DeleteAllBackups(ctx context.Context, opts ...BackupOptions) error {
	backupDir := p.getBackupDir(opts)
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

func (p *PostgreSQLBackup) ValidateBackup(ctx context.Context, backupID string, opts ...BackupOptions) error {
	if len(opts) > 0 && opts[0].Type == BackupTypeLogical {
		utils.Errorf("ValidateBackup 不支持逻辑备份，仅支持物理备份")
		return errors.New("ValidateBackup 不支持逻辑备份，仅支持物理备份")
	}

	return p.validatePhysicalBackup(ctx, backupID, opts...)
}

func init() {
	RegisterDriver(DriverMetadata{
		Name:                 "postgresql",
		Version:              "1.0.0",
		Description:          "PostgreSQL 数据库备份驱动，支持 pg_dump 逻辑备份和 pg_basebackup 物理备份",
		SupportedActions:     []string{"backup", "restore", "list", "delete", "info", "delete-all"},
		SupportedBackupTypes: []BackupType{BackupTypeLogical, BackupTypePhysical},
	}, func(config *DBConfig) (DatabaseBackup, error) {
		return NewPostgreSQLBackup(config)
	})
}
