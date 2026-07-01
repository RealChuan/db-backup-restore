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
	BackupIdentifier    string // 备份标识符（Oracle: 标签名, MSSQL/MySQL/PostgreSQL: 备份文件路径）
	TargetDatabaseName  string // 还原的目标数据库名
	Type                string // 备份类型: logical, physical
	RecoveryPointInTime string // 时间点恢复，格式: RFC3339 或 2006-01-02T15:04:05
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
	logging.Debug("=== 开始还原 ===")

	if opts.BackupIdentifier == "" && opts.RecoveryPointInTime == "" {
		return nil, fmt.Errorf("必须指定 BackupIdentifier 或 RecoveryPointInTime 参数")
	}

	if opts.BackupIdentifier == "" && dbType != "oracle" && opts.RecoveryPointInTime != "" {
		return nil, fmt.Errorf("时间点恢复仅支持 Oracle 数据库")
	}

	backupTypeVal, err := backup.ParseBackupType(opts.Type)
	if err != nil {
		logging.AuditLog(OpRestore, dbType, "failed", "无效的备份类型: "+opts.Type)
		return nil, err
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
			logging.AuditLog(OpRestore, dbType, "failed", "无效的时间格式: "+opts.RecoveryPointInTime)
			return nil, fmt.Errorf("无效的时间格式: %w", err)
		}
		restoreOpts.RecoveryPointInTime = pointInTimeVal
	}

	var result *backup.RestoreResult
	err = withDatabaseBackup(ctx, a.cfg, OpRestore, dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		var err error
		result, err = db.Restore(ctx, restoreOpts, func(percent float64, msg string) {
			logging.DebugCtx(ctx, "还原进度", "percent", fmt.Sprintf("%.2f", percent), "msg", msg)
		})
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
	}, nil
}

// notify 发送通知，如果 notifier 为 nil 则忽略
func (a *RestoreApp) notify(ctx context.Context, operation, dbType, status, message string) {
	if a.notifier != nil {
		if err := a.notifier.Notify(ctx, operation, dbType, status, message); err != nil {
			logging.Error("发送通知失败", "error", err)
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
