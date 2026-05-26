package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/RealChuan/db-backup-restore/internal/app"
	"github.com/RealChuan/db-backup-restore/internal/app/notify"
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
	RunE: func(cmd *cobra.Command, _ []string) error {
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
	var notifier *notify.Notifier
	if notifyWebhook != "" {
		notifier = notify.NewNotifier(notifyWebhook)
	}
	return app.NewBackupApp(appConfig, notifier).Run(ctx, databaseType, app.BackupOptions{
		Mode:              backupMode,
		Type:              backupType,
		ParallelWorkers:   parallelWorkers,
		EnableCompression: enableCompression,
	})
}
