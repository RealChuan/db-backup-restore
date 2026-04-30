package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"db-backup-restore/internal/backup"
	"db-backup-restore/pkg/utils"
)

var (
	backupMode        string
	parallelWorkers   int
	enableCompression bool
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "执行数据库备份",
	Long: `执行数据库备份操作，支持全量备份、增量备份等多种备份策略。

支持的备份模式(--backup-mode):
  - full:         全量备份（所有数据库支持）
  - incremental:  增量备份（仅 Oracle 支持）
  - differential: 差异备份（仅 Oracle 支持）

支持的备份类型(--backup-type):
  - logical:      逻辑备份（导出SQL文件，MySQL/PostgreSQL支持）
  - physical:     物理备份（复制数据文件，MySQL/PostgreSQL支持）

使用示例:
  # 执行 MySQL 逻辑全量备份（默认）
  db-backup-restore backup -c config.json -t mysql

  # 执行 MySQL 物理全量备份
  db-backup-restore backup -c config.json -t mysql --backup-type physical

  # 执行 Oracle 增量备份
  db-backup-restore backup -c config.json -t oracle --backup-mode incremental

  # 启用压缩和并行备份
  db-backup-restore backup -c config.json -t postgresql --enable-compression --parallel-workers 4`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBackup(cmd.Context())
	},
}

func init() {
	backupCmd.Flags().StringVar(&backupMode, "backup-mode", "full", "备份模式: full, incremental, differential")
	backupCmd.Flags().IntVar(&parallelWorkers, "parallel-workers", 2, "并行工作线程数")
	backupCmd.Flags().BoolVar(&enableCompression, "enable-compression", true, "是否启用压缩")

	rootCmd.AddCommand(backupCmd)
}

func runBackup(ctx context.Context) error {
	utils.Info("=== 开始备份 ===")

	backupModeVal, err := backup.ParseBackupMode(backupMode)
	if err != nil {
		utils.AuditLog("backup", databaseType, "failed", "无效的备份模式: "+backupMode)
		return err
	}

	backupTypeVal, err := backup.ParseBackupType(backupType)
	if err != nil {
		utils.AuditLog("backup", databaseType, "failed", "无效的备份类型: "+backupType)
		return err
	}

	backupTargetPath := filepath.Join(appConfig.BaseBackupDir, databaseType, "backup")
	archiveLogDest := filepath.Join(appConfig.BaseBackupDir, databaseType, "archivelog")

	backupOpts := backup.BackupOptions{
		Mode:              backupModeVal,
		Type:              backupTypeVal,
		ParallelWorkers:   parallelWorkers,
		EnableCompression: enableCompression,
		TargetPath:        backupTargetPath,
		ArchiveLogDest:    archiveLogDest,
		Timeout:           2 * time.Hour,
	}

	utils.Infof("备份模式: %s, 备份类型: %s", backupMode, backupType)

	return withDatabaseBackup(ctx, "backup", func(ctx context.Context, db backup.DatabaseBackup) error {
		result, err := db.Backup(ctx, backupOpts, func(percent float64, msg string) {
			utils.Infof("备份进度: %.2f%% - %s", percent, msg)
		})
		if err != nil {
			utils.AuditLog("backup", databaseType, "failed", err.Error())
			return fmt.Errorf("备份失败: %w", err)
		}

		utils.Infof("备份成功: 文件=%s, 大小=%s, 耗时=%v",
			result.BackupFile, utils.FormatFileSize(result.BackupSize), result.Duration)

		if result.Metadata["backup_set_key"] != "" {
			utils.Infof("备份集ID: %s", result.Metadata["backup_set_key"])
		}

		utils.AuditLog("backup", databaseType, "success",
			fmt.Sprintf("backup_type=%s, backup_mode=%s, file=%s, size=%d, duration=%v",
				backupType, backupMode, result.BackupFile, result.BackupSize, result.Duration))

		return nil
	})
}
