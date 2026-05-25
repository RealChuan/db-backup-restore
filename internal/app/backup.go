package app

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

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
	cfg *config.Config
}

// NewBackupApp 创建 BackupApp 实例。
func NewBackupApp(cfg *config.Config) *BackupApp {
	return &BackupApp{cfg: cfg}
}

// Run 执行备份操作。
func (a *BackupApp) Run(ctx context.Context, dbType string, opts BackupOptions) error {
	logging.Info("=== 开始备份 ===")

	backupModeVal, err := backup.ParseBackupMode(opts.Mode)
	if err != nil {
		logging.AuditLog("backup", dbType, "failed", "无效的备份模式: "+opts.Mode)
		return err
	}

	backupTypeVal, err := backup.ParseBackupType(opts.Type)
	if err != nil {
		logging.AuditLog("backup", dbType, "failed", "无效的备份类型: "+opts.Type)
		return err
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

	logging.Info(fmt.Sprintf("备份模式: %s, 备份类型: %s", opts.Mode, opts.Type))

	return withDatabaseBackup(ctx, a.cfg, "backup", dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		result, err := db.Backup(ctx, backupOpts, func(percent float64, msg string) {
			logging.Info(fmt.Sprintf("备份进度: %.2f%% - %s", percent, msg))
		})
		if err != nil {
			logging.AuditLog("backup", dbType, "failed", err.Error())
			return fmt.Errorf("备份失败: %w", err)
		}

		logging.Info(fmt.Sprintf("备份成功: 文件=%s, 大小=%s, 耗时=%v",
			result.BackupFile, FormatFileSize(result.BackupSize), result.Duration))

		if result.Metadata["backup_set_key"] != "" {
			logging.Info(fmt.Sprintf("备份集ID: %s", result.Metadata["backup_set_key"]))
		}

		logging.AuditLog("backup", dbType, "success",
			fmt.Sprintf("backup_type=%s, backup_mode=%s, file=%s, size=%d, duration=%v",
				opts.Type, opts.Mode, result.BackupFile, result.BackupSize, result.Duration))

		return nil
	})
}
