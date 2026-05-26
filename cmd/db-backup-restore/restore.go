package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/RealChuan/db-backup-restore/internal/app"
	"github.com/RealChuan/db-backup-restore/internal/app/notify"
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
	RunE: func(cmd *cobra.Command, _ []string) error {
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
	var notifier *notify.Notifier
	if notifyWebhook != "" {
		notifier = notify.NewNotifier(notifyWebhook)
	}
	return app.NewRestoreApp(appConfig, notifier).Run(ctx, databaseType, app.RestoreOptions{
		BackupIdentifier:    backupIdentifier,
		TargetDatabaseName:  targetDatabaseName,
		Type:                backupType,
		RecoveryPointInTime: recoveryPointInTime,
	})
}
