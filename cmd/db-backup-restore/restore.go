package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"db-backup-restore/internal/backup"
	"db-backup-restore/pkg/utils"
)

var (
	recoveryPointInTime string
	backupIdentifier    string
	targetDatabaseName  string
)

var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "执行数据库还原",
	Long: `执行数据库还原操作，支持按备份文件还原或时间点恢复。

使用示例:
  # 从备份文件还原 MySQL 数据库
  db-backup-restore restore -c config.json -t mysql --backup-identifier /path/to/backup.sql

  # 指定目标数据库名
  db-backup-restore restore -c config.json -t mysql --backup-identifier backup.sql --target-database new_db_name

  # 时间点恢复（Oracle 支持）
  db-backup-restore restore -c config.json -t oracle --recovery-point-in-time "2024-01-15T10:30:00"

  # PostgreSQL 从物理备份还原
  db-backup-restore restore -c config.json -t postgresql --backup-identifier /backup/postgres_backup`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRestore(cmd.Context())
	},
}

func init() {
	restoreCmd.Flags().StringVar(&recoveryPointInTime, "recovery-point-in-time", "", "时间点恢复，格式: 2006-01-02T15:04:05")
	restoreCmd.Flags().StringVar(&backupIdentifier, "backup-identifier", "", "备份标识符（Oracle: 标签名, MSSQL/MySQL/PostgreSQL: 备份文件路径）")
	restoreCmd.Flags().StringVar(&targetDatabaseName, "target-database", "", "还原的目标数据库名")

	rootCmd.AddCommand(restoreCmd)
}

func runRestore(ctx context.Context) error {
	utils.Info("=== 开始还原 ===")

	if backupIdentifier == "" && recoveryPointInTime == "" {
		return fmt.Errorf("必须指定 --backup-identifier 或 --recovery-point-in-time 参数")
	}

	if backupIdentifier == "" && databaseType != "oracle" && recoveryPointInTime != "" {
		return fmt.Errorf("时间点恢复仅支持 Oracle 数据库")
	}

	backupTypeVal, err := backup.ParseBackupType(backupType)
	if err != nil {
		utils.AuditLog("restore", databaseType, "failed", "无效的备份类型: "+backupType)
		return err
	}

	restoreOpts := backup.RestoreOptions{
		BackupIdentifier:   backupIdentifier,
		TargetDatabaseName: targetDatabaseName,
		BackupType:         backupTypeVal,
		Overwrite:          true,
	}

	if recoveryPointInTime != "" {
		pointInTimeVal, err := parseTime(recoveryPointInTime)
		if err != nil {
			utils.AuditLog("restore", databaseType, "failed", "无效的时间格式: "+recoveryPointInTime)
			return fmt.Errorf("无效的时间格式: %w", err)
		}
		restoreOpts.RecoveryPointInTime = pointInTimeVal
	}

	return withDatabaseBackup(ctx, "restore", func(ctx context.Context, db backup.DatabaseBackup) error {
		result, err := db.Restore(ctx, restoreOpts, func(percent float64, msg string) {
			utils.Infof("还原进度: %.2f%% - %s", percent, msg)
		})
		if err != nil {
			utils.AuditLog("restore", databaseType, "failed", err.Error())
			return fmt.Errorf("还原失败: %w", err)
		}

		utils.Infof("还原成功, 耗时=%v", result.Duration)

		if result.RestoredToSCN != "" {
			utils.Infof("恢复到SCN=%s", result.RestoredToSCN)
		}

		utils.AuditLog("restore", databaseType, "success",
			fmt.Sprintf("backup_tag=%s, target_db=%s, duration=%v, scn=%s",
				backupIdentifier, targetDatabaseName, result.Duration, result.RestoredToSCN))

		return nil
	})
}

func parseTime(timeStr string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02T15:04:05", timeStr)
}
