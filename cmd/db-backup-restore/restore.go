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
	remapSchema         string
	restoreMode         string
	recoverySCN         string
	recoveryLSN         string
	noRedo              bool
	archiveFromSeq      string
	archiveUntilSeq     string
)

var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "执行数据库还原",
	Long: `执行数据库还原操作，支持按备份文件还原、时间点恢复(PITR)、增量还原、归档还原等。

支持的还原模式(--restore-mode):
  - full:         全量还原（默认）
  - incremental:  增量还原（Oracle: RECOVER [NOREDO]; 达梦: RECOVER WITH BACKUPDIR）
  - archive:      归档还原（Oracle: RESTORE ARCHIVELOG; 达梦: RESTORE ARCHIVE LOG）
  - controlfile:  控制文件还原（仅 Oracle: RESTORE CONTROLFILE FROM AUTOBACKUP）

还原目标指定:
  - --backup-identifier: 指定备份标识符（Oracle/达梦: TAG 或备份集路径; MySQL/PostgreSQL/MSSQL: 文件路径）
  - --recovery-point-in-time: 时间点还原（Oracle/达梦支持）
  - --scn: 按 SCN 还原（仅 Oracle）
  - --lsn: 按 LSN 还原（仅达梦）

使用示例:
  # MySQL 逻辑还原
  db-backup-restore restore -c config.json -t mysql --backup-identifier /path/to/backup.sql

  # MySQL 物理还原
  db-backup-restore restore -c config.json -t mysql --backup-type physical --backup-identifier /path/to/backup

  # MySQL 还原到指定数据库
  db-backup-restore restore -c config.json -t mysql --backup-identifier backup.sql --target-database new_db_name

  # Oracle 全量还原（默认模式）
  db-backup-restore restore -c config.json -t oracle --backup-type physical --backup-identifier /path/to/backup

  # Oracle 指定 TAG 还原
  db-backup-restore restore -c config.json -t oracle --backup-type physical --backup-identifier TAG20260703T120000

  # Oracle 时间点还原（PITR）
  db-backup-restore restore -c config.json -t oracle --backup-type physical --recovery-point-in-time "2024-01-15T10:30:00"

  # Oracle 按 TAG + 时间点组合还原
  db-backup-restore restore -c config.json -t oracle --backup-type physical \
    --backup-identifier TAG20260703T120000 --recovery-point-in-time "2024-01-15T10:30:00"

  # Oracle 按 SCN 还原
  db-backup-restore restore -c config.json -t oracle --backup-type physical --scn 123456789

  # Oracle 按 TAG + SCN 组合还原
  db-backup-restore restore -c config.json -t oracle --backup-type physical \
    --backup-identifier TAG20260703T120000 --scn 123456789

  # Oracle 增量还原（NOREDO 模式，跳过归档日志应用）
  db-backup-restore restore -c config.json -t oracle --backup-type physical \
    --restore-mode incremental --no-redo

  # Oracle 归档还原（按序列号范围）
  db-backup-restore restore -c config.json -t oracle --backup-type physical \
    --restore-mode archive --archive-from-seq 100 --archive-until-seq 200

  # Oracle 控制文件还原（控制文件丢失的灾难恢复）
  db-backup-restore restore -c config.json -t oracle --backup-type physical --restore-mode controlfile

  # Oracle 控制文件还原（指定 TAG）
  db-backup-restore restore -c config.json -t oracle --backup-type physical \
    --restore-mode controlfile --backup-identifier TAG20260703T120000

  # 达梦全量还原
  db-backup-restore restore -c config.json -t dameng --backup-type physical --backup-identifier /backup/dm_full

  # 达梦增量还原
  db-backup-restore restore -c config.json -t dameng --backup-type physical \
    --restore-mode incremental --backup-identifier /backup/dm_incr

  # 达梦时间点还原
  db-backup-restore restore -c config.json -t dameng --backup-type physical \
    --recovery-point-in-time "2024-01-15T10:30:00"

  # 达梦按 LSN 还原
  db-backup-restore restore -c config.json -t dameng --backup-type physical \
    --restore-mode archive --lsn 99999

  # PostgreSQL 物理还原
  db-backup-restore restore -c config.json -t postgresql --backup-type physical --backup-identifier /backup/pg_backup`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runRestore(cmd.Context())
	},
}

func init() {
	restoreCmd.Flags().StringVar(&backupIdentifier, "backup-identifier", "",
		"备份标识符（Oracle/达梦: TAG 或备份集路径; MySQL/PostgreSQL/MSSQL: 备份文件路径）")
	restoreCmd.Flags().StringVar(&targetDatabaseName, "target-database", "",
		"还原的目标数据库名（MySQL/PostgreSQL/MSSQL 逻辑还原时指定）")
	restoreCmd.Flags().StringVar(&remapSchema, "remap-schema", "",
		"模式映射，格式: source:target（仅达梦 dimp 支持，将源模式数据导入目标模式）")
	restoreCmd.Flags().StringVar(&restoreMode, "restore-mode", "full",
		"还原模式: full(全量), incremental(增量), archive(归档), controlfile(控制文件,仅Oracle)")
	restoreCmd.Flags().StringVar(&recoveryPointInTime, "recovery-point-in-time", "",
		"时间点还原，格式: 2006-01-02T15:04:05（Oracle/达梦支持，可与 --backup-identifier 组合）")
	restoreCmd.Flags().StringVar(&recoverySCN, "scn", "",
		"按 SCN 还原（仅 Oracle 支持，可与 --backup-identifier 组合）")
	restoreCmd.Flags().StringVar(&recoveryLSN, "lsn", "",
		"按 LSN 还原（仅达梦支持，配合 --restore-mode archive 使用）")
	restoreCmd.Flags().BoolVar(&noRedo, "no-redo", false,
		"增量还原时跳过归档日志应用，即 NOREDO（仅 Oracle 支持）")
	restoreCmd.Flags().StringVar(&archiveFromSeq, "archive-from-seq", "",
		"归档还原起始序列号（仅 Oracle 支持，配合 --restore-mode archive 使用）")
	restoreCmd.Flags().StringVar(&archiveUntilSeq, "archive-until-seq", "",
		"归档还原结束序列号（仅 Oracle 支持，配合 --restore-mode archive 使用）")

	rootCmd.AddCommand(restoreCmd)
}

func runRestore(ctx context.Context) error {
	var notifier *notify.Notifier
	if notifyWebhook != "" {
		notifier = notify.NewNotifier(notifyWebhook)
	}
	result, err := app.NewRestoreApp(appConfig, notifier).Run(ctx, databaseType, app.RestoreOptions{
		BackupIdentifier:    backupIdentifier,
		TargetDatabaseName:  targetDatabaseName,
		RemapSchema:         remapSchema,
		Type:                backupType,
		RecoveryPointInTime: recoveryPointInTime,
		RestoreMode:         restoreMode,
		RecoverySCN:         recoverySCN,
		RecoveryLSN:         recoveryLSN,
		NoRedo:              noRedo,
		ArchiveFromSeq:      archiveFromSeq,
		ArchiveUntilSeq:     archiveUntilSeq,
	})
	return outputResult(result, err, "restore")
}
