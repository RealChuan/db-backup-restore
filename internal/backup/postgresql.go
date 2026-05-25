package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RealChuan/db-backup-restore/internal/logging"
	"github.com/RealChuan/db-backup-restore/pkg/fileutil"
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
	fsManager          *FileSystemBackupManager
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

	if val := config.GetExtraTyped().PGBinPath(); val != "" {
		psqlPath = fileutil.AddExeExt(filepath.Join(val, "psql"))
		pgDumpPath = fileutil.AddExeExt(filepath.Join(val, "pg_dump"))
		pgDumpallPath = fileutil.AddExeExt(filepath.Join(val, "pg_dumpall"))
		pgBasebackupPath = fileutil.AddExeExt(filepath.Join(val, "pg_basebackup"))
		pgVerifyBackupPath = fileutil.AddExeExt(filepath.Join(val, "pg_verifybackup"))
		pgCtlPath = fileutil.AddExeExt(filepath.Join(val, "pg_ctl"))
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
		fsManager:          NewFileSystemBackupManager("", "postgresql", nil),
	}, nil
}

func (p *PostgreSQLBackup) Backup(ctx context.Context, opts BackupOptions, callback ProgressCallback) (*BackupResult, error) {
	if opts.Mode == BackupModeIncremental || opts.Mode == BackupModeDifferential {
		logging.Info("PostgreSQL 不支持增量/差异备份模式，将使用全量备份")
	}

	backupDir := opts.TargetPath
	if backupDir == "" {
		return nil, errors.New("必须通过 -target-path 参数指定备份路径")
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return nil, err
	}

	databaseName := p.config.Database
	databases := p.parseDatabaseNames(databaseName)

	switch opts.Type {
	case BackupTypePhysical:
		if len(databases) > 0 {
			logging.Warn(fmt.Sprintf("物理备份将备份整个 PostgreSQL 实例，指定的数据库列表 [%s] 将被忽略", strings.Join(databases, ", ")))
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

// ListBackups 列出所有备份（委托给 FileSystemBackupManager）
func (p *PostgreSQLBackup) ListBackups(ctx context.Context, opts ...BackupOptions) ([]BackupInfo, error) {
	return p.fsManager.ListBackups(ctx, p.getBackupDir(opts))
}

// DeleteBackup 删除指定备份（委托给 FileSystemBackupManager）
func (p *PostgreSQLBackup) DeleteBackup(ctx context.Context, identifier string, opts ...BackupOptions) error {
	return p.fsManager.DeleteBackup(ctx, identifier, p.getBackupDir(opts))
}

// GetBackupInfo 获取指定备份的详细信息（委托给 FileSystemBackupManager）
func (p *PostgreSQLBackup) GetBackupInfo(ctx context.Context, backupID string, opts ...BackupOptions) (map[string]string, error) {
	return p.fsManager.GetBackupInfo(ctx, backupID, p.getBackupDir(opts))
}

// DeleteAllBackups 删除所有备份（委托给 FileSystemBackupManager）
func (p *PostgreSQLBackup) DeleteAllBackups(ctx context.Context, opts ...BackupOptions) error {
	return p.fsManager.DeleteAllBackups(ctx, p.getBackupDir(opts))
}

func (p *PostgreSQLBackup) ValidateBackup(ctx context.Context, backupID string, opts ...BackupOptions) error {
	if len(opts) > 0 && opts[0].Type == BackupTypeLogical {
		logging.Error("ValidateBackup 不支持逻辑备份，仅支持物理备份")
		return errors.New("ValidateBackup 不支持逻辑备份，仅支持物理备份")
	}

	return p.validatePhysicalBackup(ctx, backupID, opts...)
}

func registerPostgreSQLDriver() {
	RegisterDriver(DriverMetadata{
		Name:                 DBTypePostgreSQL,
		Version:              versionXML,
		Description:          "PostgreSQL 数据库备份驱动，支持 pg_dump 逻辑备份和 pg_basebackup 物理备份",
		SupportedActions:     []string{backupTypeXML, actionRestore, actionList, actionDelete, actionInfo, actionDeleteAll},
		SupportedBackupTypes: []BackupType{BackupTypeLogical, BackupTypePhysical},
	}, func(config *DBConfig) (DatabaseBackup, error) {
		return NewPostgreSQLBackup(config)
	})
}
