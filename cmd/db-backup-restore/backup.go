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
	compressionLevel  int
	encryption        bool
	encryptionKey     string
	archiveFromLSN    string
	archiveUntilLSN   string
	retentionDays     int
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "执行数据库备份",
	Long: `执行数据库备份操作，支持全量备份、增量备份、归档日志备份等多种策略。

支持的备份模式(--backup-mode):
  - full:         全量备份（所有数据库支持）
  - incremental:  差异增量备份（Oracle: LEVEL 1; 达梦: INCREMENT）
  - differential: 累积增量备份（Oracle: LEVEL 1 CUMULATIVE; 达梦: INCREMENT CUMULATIVE）
  - level0:       Level 0 增量基础备份（仅 Oracle: LEVEL 0，作为增量策略的基础）
  - archive:      独立归档日志备份（Oracle/达梦支持，不含数据文件备份）

支持的备份类型(--backup-type):
  - logical:      逻辑备份（导出SQL文件，MySQL/PostgreSQL/达梦支持）
  - physical:     物理备份（复制数据文件，MySQL/PostgreSQL/Oracle/达梦支持）

使用示例:
  # MySQL 逻辑全量备份（默认）
  db-backup-restore backup -c config.json -t mysql

  # MySQL 物理全量备份
  db-backup-restore backup -c config.json -t mysql --backup-type physical

  # Oracle 全量物理备份（启用压缩和加密）
  db-backup-restore backup -c config.json -t oracle --backup-type physical \
    --enable-compression --encryption --encryption-key mypassword

  # Oracle Level 0 增量基础备份（增量策略的起点）
  db-backup-restore backup -c config.json -t oracle --backup-type physical --backup-mode level0

  # Oracle 差异增量备份
  db-backup-restore backup -c config.json -t oracle --backup-type physical --backup-mode incremental

  # Oracle 累积增量备份
  db-backup-restore backup -c config.json -t oracle --backup-type physical --backup-mode differential

  # 达梦全量物理备份（启用并行和压缩）
  db-backup-restore backup -c config.json -t dameng --backup-type physical \
    --enable-compression --parallel-workers 4

  # 达梦差异增量备份
  db-backup-restore backup -c config.json -t dameng --backup-type physical --backup-mode incremental

  # 达梦累积增量备份
  db-backup-restore backup -c config.json -t dameng --backup-type physical --backup-mode differential

  # 达梦独立归档日志备份（按 LSN 范围，启用加密）
  db-backup-restore backup -c config.json -t dameng --backup-type physical \
    --backup-mode archive --archive-from-lsn 1000 --archive-until-lsn 5000 \
    --encryption --encryption-key mypassword

  # PostgreSQL 物理备份（启用压缩和并行）
  db-backup-restore backup -c config.json -t postgresql --backup-type physical \
    --enable-compression --parallel-workers 4`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runBackup(cmd.Context())
	},
}

func init() {
	backupCmd.Flags().StringVar(&backupMode, "backup-mode", "full",
		"备份模式: full(全量), incremental(差异增量), differential(累积增量), level0(Oracle增量基础), archive(独立归档日志)")
	backupCmd.Flags().IntVar(&parallelWorkers, "parallel-workers", 2, "并行工作线程数（物理备份生效）")
	backupCmd.Flags().BoolVar(&enableCompression, "enable-compression", true, "是否启用压缩")
	backupCmd.Flags().IntVar(&compressionLevel, "compression-level", 0,
		"压缩级别，仅物理备份生效（0=默认; 达梦: 1-9; Oracle: 1-3=LOW, 4-6=MEDIUM, 7-9=HIGH）")
	backupCmd.Flags().BoolVar(&encryption, "encryption", false, "是否启用加密（物理备份，Oracle/达梦支持）")
	backupCmd.Flags().StringVar(&encryptionKey, "encryption-key", "", "加密密钥（需配合 --encryption 使用）")
	backupCmd.Flags().StringVar(&archiveFromLSN, "archive-from-lsn", "",
		"归档备份起始 LSN（仅达梦: 配合 --backup-mode archive 使用）")
	backupCmd.Flags().StringVar(&archiveUntilLSN, "archive-until-lsn", "",
		"归档备份结束 LSN（仅达梦: 配合 --backup-mode archive 使用）")
	backupCmd.Flags().IntVar(&retentionDays, "retention-days", 7,
		"增量策略保留窗口天数（仅 Oracle 支持，默认 7，配合 incremental/differential/level0 模式使用）")

	rootCmd.AddCommand(backupCmd)
}

func runBackup(ctx context.Context) error {
	var notifier *notify.Notifier
	if notifyWebhook != "" {
		notifier = notify.NewNotifier(notifyWebhook)
	}
	result, err := app.NewBackupApp(appConfig, notifier).Run(ctx, databaseType, app.BackupOptions{
		Mode:              backupMode,
		Type:              backupType,
		ParallelWorkers:   parallelWorkers,
		EnableCompression: enableCompression,
		CompressionLevel:  compressionLevel,
		Encryption:        encryption,
		EncryptionKey:     encryptionKey,
		ArchiveFromLSN:    archiveFromLSN,
		ArchiveUntilLSN:   archiveUntilLSN,
		RetentionDays:     retentionDays,
	})
	return outputResult(result, err, "backup")
}
