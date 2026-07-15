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

// BackupOptions 备份应用层的选项参数。
type BackupOptions struct {
	Mode            string // 备份模式: full, incremental, differential, level0, archive
	Type            string // 备份类型: logical, physical
	Encryption      bool   // 是否启用加密（物理备份，Oracle/达梦支持）
	EncryptionKey   string // 加密密钥（需配合 Encryption 使用）
	ArchiveFromLSN  string // 归档备份起始 LSN（仅达梦: 配合 archive 模式使用）
	ArchiveUntilLSN string // 归档备份结束 LSN（仅达梦: 配合 archive 模式使用）
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
	logging.InfoCtx(ctx, "开始备份", "db_type", dbType, "mode", opts.Mode, "type", opts.Type)

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

	if opts.Encryption && opts.EncryptionKey == "" {
		return nil, fmt.Errorf("启用加密时必须提供 --encryption-key")
	}

	// typeDir 由 ParseBackupType 保证非空（空值默认为 logical）。
	typeDir := string(backupTypeVal)

	backupOpts := backup.BackupOptions{
		Mode:            backupModeVal,
		Type:            backupTypeVal,
		Encryption:      opts.Encryption,
		EncryptionKey:   opts.EncryptionKey,
		TargetPath:      backupDir(a.cfg.BaseBackupDir, dbType, typeDir),
		ArchiveLogDest:  archiveLogDir(a.cfg.BaseBackupDir, dbType, typeDir),
		ArchiveFromLSN:  opts.ArchiveFromLSN,
		ArchiveUntilLSN: opts.ArchiveUntilLSN,
		Timeout:         2 * time.Hour,
	}

	var result *backup.BackupResult
	err = withDatabaseBackup(ctx, a.cfg, OpBackup, dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		var err error
		result, err = db.Backup(ctx, backupOpts, nil)
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
		Message:   MsgBackupSuccess,
		Data:      data,
	}, nil
}

// notify 发送通知，如果 notifier 为 nil 则忽略
func (a *BackupApp) notify(ctx context.Context, operation, dbType, status, message string) {
	if a.notifier != nil {
		if err := a.notifier.Notify(ctx, operation, dbType, status, message); err != nil {
			logging.ErrorCtx(ctx, "发送通知失败", "error", err)
		}
	}
}
