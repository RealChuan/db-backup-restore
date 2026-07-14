package app

import (
	"context"
	"fmt"
	"time"

	"github.com/RealChuan/db-backup-restore/internal/app/notify"
	"github.com/RealChuan/db-backup-restore/internal/backup"
	"github.com/RealChuan/db-backup-restore/internal/config"
	"github.com/RealChuan/db-backup-restore/internal/logging"
)

// RestoreOptions 还原应用层的选项参数。
type RestoreOptions struct {
	BackupIdentifier       string // 备份标识符（Oracle/达梦: TAG 或备份集路径; MySQL/PostgreSQL/MSSQL: 备份文件路径）
	TargetDatabaseName     string // 还原的目标数据库名（MySQL/PostgreSQL/MSSQL 逻辑还原时指定）
	RemapSchema            string // 模式映射（达梦 dimp: REMAP_SCHEMA=source:target，将源模式数据导入目标模式）
	Type                   string // 备份类型: logical, physical
	RecoveryPointInTime    string // 时间点还原，格式: 2006-01-02T15:04:05（Oracle/达梦支持，可与 BackupIdentifier 组合）
	RestoreMode            string // 还原模式: full, incremental, archive, controlfile（Oracle/达梦支持）
	RecoverySCN            string // 按 SCN 还原（仅 Oracle 支持，可与 BackupIdentifier 组合）
	RecoveryLSN            string // 按 LSN 还原（仅达梦支持，配合 archive 模式使用）
	NoRedo                 bool   // 增量还原时跳过归档日志应用，即 NOREDO（仅 Oracle 支持）
	ArchiveFromSeq         string // 归档还原起始序列号（仅 Oracle 支持，配合 archive 模式使用）
	ArchiveUntilSeq        string // 归档还原结束序列号（仅 Oracle 支持，配合 archive 模式使用）
	CatalogPath            string // 备份文件注册路径（仅 Oracle 支持，异机还原时使用 CATALOG START WITH 注册备份）
	AutoRestoreControlFile bool   // 自动恢复控制文件（仅 Oracle 支持，在数据库还原流程中先恢复控制文件）
}

// RestoreApp 封装还原操作的应用服务。
type RestoreApp struct {
	cfg      *config.Config
	notifier *notify.Notifier
}

// NewRestoreApp 创建 RestoreApp 实例。
func NewRestoreApp(cfg *config.Config, notifier *notify.Notifier) *RestoreApp {
	return &RestoreApp{cfg: cfg, notifier: notifier}
}

// Run 执行还原操作。
func (a *RestoreApp) Run(ctx context.Context, dbType string, opts RestoreOptions) (*OperationResult, error) {
	logging.InfoCtx(ctx, "开始还原", "db_type", dbType, "type", opts.Type)

	restoreOpts, err := a.buildRestoreOptions(dbType, opts)
	if err != nil {
		return nil, err
	}

	var result *backup.RestoreResult
	err = withDatabaseBackup(ctx, a.cfg, OpRestore, dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		var err error
		result, err = db.Restore(ctx, restoreOpts, nil)
		if err != nil {
			logging.AuditLog(OpRestore, dbType, "failed", err.Error())
			a.notify(ctx, OpRestore, dbType, "failed", err.Error())
			return fmt.Errorf("还原失败: %w", err)
		}

		logging.AuditLog(OpRestore, dbType, "success",
			fmt.Sprintf("backup_tag=%s, target_db=%s, duration=%v, scn=%s",
				opts.BackupIdentifier, opts.TargetDatabaseName, result.Duration, result.RestoredToSCN))

		return nil
	})
	if err != nil {
		return nil, err
	}

	return a.buildRestoreResult(dbType, result), nil
}

// buildRestoreOptions 校验参数并构建 backup.RestoreOptions。
func (a *RestoreApp) buildRestoreOptions(dbType string, opts RestoreOptions) (backup.RestoreOptions, error) {
	if opts.BackupIdentifier == "" && opts.RecoveryPointInTime == "" {
		return backup.RestoreOptions{}, fmt.Errorf("必须指定 BackupIdentifier 或 RecoveryPointInTime 参数")
	}

	if opts.RecoveryPointInTime != "" && dbType != "oracle" && dbType != "dameng" {
		return backup.RestoreOptions{}, fmt.Errorf("时间点还原仅支持 Oracle 和达梦数据库")
	}

	backupTypeVal, err := backup.ParseBackupType(opts.Type)
	if err != nil {
		logging.AuditLog(OpRestore, dbType, "failed", "无效的备份类型: "+opts.Type)
		return backup.RestoreOptions{}, err
	}

	restoreModeVal, err := backup.ParseRestoreMode(opts.RestoreMode)
	if err != nil {
		logging.AuditLog(OpRestore, dbType, "failed", "无效的还原模式: "+opts.RestoreMode)
		return backup.RestoreOptions{}, err
	}

	// typeDir 由 ParseBackupType 保证非空（空值默认为 logical）。
	typeDir := string(backupTypeVal)

	restoreOpts := backup.RestoreOptions{
		BackupIdentifier:       opts.BackupIdentifier,
		TargetDatabaseName:     opts.TargetDatabaseName,
		RemapSchema:            opts.RemapSchema,
		BackupType:             backupTypeVal,
		RestoreMode:            restoreModeVal,
		RecoverySCN:            opts.RecoverySCN,
		RecoveryLSN:            opts.RecoveryLSN,
		NoRedo:                 opts.NoRedo,
		ArchiveFromSeq:         opts.ArchiveFromSeq,
		ArchiveUntilSeq:        opts.ArchiveUntilSeq,
		CatalogPath:            opts.CatalogPath,
		AutoRestoreControlFile: opts.AutoRestoreControlFile,
		ArchiveLogDest:         archiveLogDir(a.cfg.BaseBackupDir, dbType, typeDir),
	}

	if opts.RecoveryPointInTime != "" {
		pointInTimeVal, err := parseTime(opts.RecoveryPointInTime)
		if err != nil {
			logging.AuditLog(OpRestore, dbType, "failed", "无效的时间格式: "+opts.RecoveryPointInTime)
			return backup.RestoreOptions{}, fmt.Errorf("无效的时间格式: %w", err)
		}
		restoreOpts.RecoveryPointInTime = pointInTimeVal
	}

	return restoreOpts, nil
}

// buildRestoreResult 构建还原操作的返回结果。
func (a *RestoreApp) buildRestoreResult(dbType string, result *backup.RestoreResult) *OperationResult {
	data := map[string]interface{}{
		DataKeyDuration: result.Duration.String(),
	}
	if result.TargetDatabase != "" {
		data[DataKeyTargetDB] = result.TargetDatabase
	}
	if result.RestoredToSCN != "" {
		data[DataKeySCN] = result.RestoredToSCN
	}

	return &OperationResult{
		Success:   true,
		Operation: OpRestore,
		DBType:    dbType,
		Message:   "还原成功",
		Data:      data,
	}
}

// notify 发送通知，如果 notifier 为 nil 则忽略
func (a *RestoreApp) notify(ctx context.Context, operation, dbType, status, message string) {
	if a.notifier != nil {
		if err := a.notifier.Notify(ctx, operation, dbType, status, message); err != nil {
			logging.ErrorCtx(ctx, "发送通知失败", "error", err)
		}
	}
}

// parseTime 解析时间字符串，支持 RFC3339 和 "2006-01-02T15:04:05" 格式。
func parseTime(timeStr string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02T15:04:05", timeStr)
}
