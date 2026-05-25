package app

import (
	"context"
	"fmt"
	"time"

	"github.com/RealChuan/db-backup-restore/internal/backup"
	"github.com/RealChuan/db-backup-restore/internal/config"
	"github.com/RealChuan/db-backup-restore/internal/logging"
)

// RestoreOptions 还原应用层的选项参数。
type RestoreOptions struct {
	BackupIdentifier    string // 备份标识符（Oracle: 标签名, MSSQL/MySQL/PostgreSQL: 备份文件路径）
	TargetDatabaseName  string // 还原的目标数据库名
	Type                string // 备份类型: logical, physical
	RecoveryPointInTime string // 时间点恢复，格式: RFC3339 或 2006-01-02T15:04:05
}

// RestoreApp 封装还原操作的应用服务。
type RestoreApp struct {
	cfg *config.Config
}

// NewRestoreApp 创建 RestoreApp 实例。
func NewRestoreApp(cfg *config.Config) *RestoreApp {
	return &RestoreApp{cfg: cfg}
}

// Run 执行还原操作。
func (a *RestoreApp) Run(ctx context.Context, dbType string, opts RestoreOptions) error {
	logging.Info("=== 开始还原 ===")

	if opts.BackupIdentifier == "" && opts.RecoveryPointInTime == "" {
		return fmt.Errorf("必须指定 BackupIdentifier 或 RecoveryPointInTime 参数")
	}

	if opts.BackupIdentifier == "" && dbType != "oracle" && opts.RecoveryPointInTime != "" {
		return fmt.Errorf("时间点恢复仅支持 Oracle 数据库")
	}

	backupTypeVal, err := backup.ParseBackupType(opts.Type)
	if err != nil {
		logging.AuditLog("restore", dbType, "failed", "无效的备份类型: "+opts.Type)
		return err
	}

	restoreOpts := backup.RestoreOptions{
		BackupIdentifier:   opts.BackupIdentifier,
		TargetDatabaseName: opts.TargetDatabaseName,
		BackupType:         backupTypeVal,
		Overwrite:          true,
	}

	if opts.RecoveryPointInTime != "" {
		pointInTimeVal, err := parseTime(opts.RecoveryPointInTime)
		if err != nil {
			logging.AuditLog("restore", dbType, "failed", "无效的时间格式: "+opts.RecoveryPointInTime)
			return fmt.Errorf("无效的时间格式: %w", err)
		}
		restoreOpts.RecoveryPointInTime = pointInTimeVal
	}

	return withDatabaseBackup(ctx, a.cfg, "restore", dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		result, err := db.Restore(ctx, restoreOpts, func(percent float64, msg string) {
			logging.Info(fmt.Sprintf("还原进度: %.2f%% - %s", percent, msg))
		})
		if err != nil {
			logging.AuditLog("restore", dbType, "failed", err.Error())
			return fmt.Errorf("还原失败: %w", err)
		}

		logging.Info(fmt.Sprintf("还原成功, 耗时=%v", result.Duration))

		if result.RestoredToSCN != "" {
			logging.Info(fmt.Sprintf("恢复到SCN=%s", result.RestoredToSCN))
		}

		logging.AuditLog("restore", dbType, "success",
			fmt.Sprintf("backup_tag=%s, target_db=%s, duration=%v, scn=%s",
				opts.BackupIdentifier, opts.TargetDatabaseName, result.Duration, result.RestoredToSCN))

		return nil
	})
}

// parseTime 解析时间字符串，支持 RFC3339 和 "2006-01-02T15:04:05" 格式。
func parseTime(timeStr string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02T15:04:05", timeStr)
}
