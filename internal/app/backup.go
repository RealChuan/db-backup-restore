package app

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/RealChuan/db-backup-restore/internal/app/notify"
	"github.com/RealChuan/db-backup-restore/internal/backup"
	"github.com/RealChuan/db-backup-restore/internal/config"
	"github.com/RealChuan/db-backup-restore/internal/logging"
)

// BackupOptions 备份应用层的选项参数。
type BackupOptions struct {
	Mode              string // 备份模式: full, incremental, differential
	Type              string // 备份类型: logical, physical
	ParallelWorkers   int    // 并行工作线程数
	EnableCompression bool   // 是否启用压缩
}

// BackupApp 封装备份操作的应用服务。
type BackupApp struct {
	cfg      *config.Config
	notifier *notify.Notifier
}

// NewBackupApp 创建 BackupApp 实例。
func NewBackupApp(cfg *config.Config, notifier *notify.Notifier) *BackupApp {
	return &BackupApp{cfg: cfg, notifier: notifier}
}

// Run 执行备份操作。
func (a *BackupApp) Run(ctx context.Context, dbType string, opts BackupOptions) (*OperationResult, error) {
	logging.Debug("=== 开始备份 ===")

	backupModeVal, err := backup.ParseBackupMode(opts.Mode)
	if err != nil {
		logging.AuditLog(OpBackup, dbType, "failed", "无效的备份模式: "+opts.Mode)
		return nil, err
	}

	backupTypeVal, err := backup.ParseBackupType(opts.Type)
	if err != nil {
		logging.AuditLog(OpBackup, dbType, "failed", "无效的备份类型: "+opts.Type)
		return nil, err
	}

	backupTargetPath := filepath.Join(a.cfg.BaseBackupDir, dbType, "backup")
	archiveLogDest := filepath.Join(a.cfg.BaseBackupDir, dbType, "archivelog")

	backupOpts := backup.BackupOptions{
		Mode:              backupModeVal,
		Type:              backupTypeVal,
		ParallelWorkers:   opts.ParallelWorkers,
		EnableCompression: opts.EnableCompression,
		TargetPath:        backupTargetPath,
		ArchiveLogDest:    archiveLogDest,
		Timeout:           2 * time.Hour,
	}

	logging.DebugCtx(ctx, "备份模式与类型", "mode", opts.Mode, "type", opts.Type)

	var result *backup.BackupResult
	err = withDatabaseBackup(ctx, a.cfg, OpBackup, dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		var err error
		result, err = db.Backup(ctx, backupOpts, func(percent float64, msg string) {
			logging.DebugCtx(ctx, "备份进度", "percent", fmt.Sprintf("%.2f", percent), "msg", msg)
		})
		if err != nil {
			logging.AuditLog(OpBackup, dbType, "failed", err.Error())
			a.notify(ctx, OpBackup, dbType, "failed", err.Error())
			return fmt.Errorf("备份失败: %w", err)
		}

		logging.AuditLog(OpBackup, dbType, "success",
			fmt.Sprintf("backup_type=%s, backup_mode=%s, file=%s, size=%d, duration=%v",
				opts.Type, opts.Mode, result.BackupFile, result.BackupSize, result.Duration))

		return nil
	})
	if err != nil {
		return nil, err
	}

	data := map[string]interface{}{
		DataKeyFile:     result.BackupFile,
		DataKeySize:     FormatFileSize(result.BackupSize),
		DataKeyDuration: result.Duration.String(),
	}
	if result.Metadata["backup_set_key"] != "" {
		data[DataKeyBackupSetKey] = result.Metadata["backup_set_key"]
	}

	return &OperationResult{
		Success:   true,
		Operation: OpBackup,
		DBType:    dbType,
		Message:   "备份成功",
		Data:      data,
	}, nil
}

// notify 发送通知，如果 notifier 为 nil 则忽略
func (a *BackupApp) notify(ctx context.Context, operation, dbType, status, message string) {
	if a.notifier != nil {
		if err := a.notifier.Notify(ctx, operation, dbType, status, message); err != nil {
			logging.Error("发送通知失败", "error", err)
		}
	}
}
