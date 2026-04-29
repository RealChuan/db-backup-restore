package main

import (
	"context"
	"errors"
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
	Long: `执行数据库备份操作，支持全量备份、增量备份等多种备份类型。

支持的备份模式:
  - full:         全量备份（所有数据库支持）
  - incremental:  增量备份（仅 Oracle 支持）
  - differential: 差异备份（仅 Oracle 支持）
  - logical:      逻辑备份（MySQL/PostgreSQL 支持）
  - physical:     物理备份（仅 PostgreSQL 支持）

使用示例:
  # 执行 MySQL 全量备份
  db-backup-restore backup -c config.json -t mysql

  # 执行 Oracle 增量备份
  db-backup-restore backup -c config.json -t oracle --backup-mode incremental

  # 启用压缩和并行备份
  db-backup-restore backup -c config.json -t postgresql --enable-compression --parallel-workers 4`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBackup(cmd.Context())
	},
}

func init() {
	backupCmd.Flags().StringVar(&backupMode, "backup-mode", "full", "备份模式: full, incremental, differential, logical, physical")
	backupCmd.Flags().IntVar(&parallelWorkers, "parallel-workers", 2, "并行工作线程数")
	backupCmd.Flags().BoolVar(&enableCompression, "enable-compression", true, "是否启用压缩")

	rootCmd.AddCommand(backupCmd)
}

func runBackup(ctx context.Context) error {
	utils.Info("=== 开始备份 ===")

	backupType, err := parseBackupType(backupMode)
	if err != nil {
		utils.AuditLog("backup", databaseType, "failed", "无效的备份类型: "+backupMode)
		return err
	}

	backupTargetPath := filepath.Join(appConfig.BaseBackupDir, databaseType, "backup")
	archiveLogDest := filepath.Join(appConfig.BaseBackupDir, databaseType, "archivelog")

	backupOpts := backup.BackupOptions{
		Type:              backupType,
		ParallelWorkers:   parallelWorkers,
		EnableCompression: enableCompression,
		TargetPath:        backupTargetPath,
		ArchiveLogDest:    archiveLogDest,
		Timeout:           2 * time.Hour,
	}

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
			fmt.Sprintf("backup_type=%s, file=%s, size=%d, duration=%v",
				backupMode, result.BackupFile, result.BackupSize, result.Duration))

		return nil
	})
}

func parseBackupType(backupType string) (backup.BackupType, error) {
	switch backupType {
	case "full":
		return backup.BackupFull, nil
	case "incremental":
		return backup.BackupIncremental, nil
	case "differential":
		return backup.BackupDifferential, nil
	case "logical":
		return backup.BackupLogical, nil
	case "physical":
		return backup.BackupPhysical, nil
	default:
		return "", errors.New("无效的备份类型: " + backupType)
	}
}
