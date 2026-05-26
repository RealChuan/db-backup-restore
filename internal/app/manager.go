package app

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RealChuan/db-backup-restore/internal/backup"
	"github.com/RealChuan/db-backup-restore/internal/config"
	"github.com/RealChuan/db-backup-restore/internal/logging"
	"github.com/RealChuan/db-backup-restore/pkg/fileutil"
)

// ManagerApp 封装备份管理操作的应用服务。
type ManagerApp struct {
	cfg *config.Config
}

// NewManagerApp 创建 ManagerApp 实例。
func NewManagerApp(cfg *config.Config) *ManagerApp {
	return &ManagerApp{cfg: cfg}
}

// withDatabaseBackup 封装数据库备份实例的创建和释放逻辑。
func withDatabaseBackup(ctx context.Context, cfg *config.Config, operation, dbType string, fn func(ctx context.Context, db backup.DatabaseBackup) error) error {
	dbCfg, err := cfg.GetDBConfig(dbType)
	if err != nil {
		logging.AuditLog(operation, dbType, "failed", "获取数据库配置失败: "+err.Error())
		return fmt.Errorf("获取数据库配置失败: %w", err)
	}

	db, err := backup.NewBackup(dbCfg)
	if err != nil {
		logging.AuditLog(operation, dbType, "failed", "创建数据库备份实例失败: "+err.Error())
		return fmt.Errorf("创建数据库备份实例失败: %w", err)
	}
	defer db.Close()

	return fn(ctx, db)
}

// ListBackups 列出指定数据库的所有备份。
func (m *ManagerApp) ListBackups(ctx context.Context, dbType string) error {
	logging.Info("=== 列出所有备份 ===")

	backupTarget := filepath.Join(m.cfg.BaseBackupDir, dbType, "backup")
	backupOpts := backup.BackupOptions{TargetPath: backupTarget}

	return withDatabaseBackup(ctx, m.cfg, "list", dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		backups, err := db.ListBackups(ctx, backupOpts)
		if err != nil {
			return fmt.Errorf("列出备份失败: %w", err)
		}

		if len(backups) == 0 {
			logging.Info("未找到备份")
			return nil
		}

		for _, b := range backups {
			logBackupInfo(b)
		}

		return nil
	})
}

// DeleteBackup 删除指定标识符的备份。
func (m *ManagerApp) DeleteBackup(ctx context.Context, dbType, identifier string) error {
	logging.InfoCtx(ctx, "删除备份", "identifier", identifier)

	backupTarget := filepath.Join(m.cfg.BaseBackupDir, dbType, "backup")
	backupOpts := backup.BackupOptions{TargetPath: backupTarget}

	return withDatabaseBackup(ctx, m.cfg, "delete", dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		if err := db.DeleteBackup(ctx, identifier, backupOpts); err != nil {
			logging.AuditLog("delete", dbType, "failed", "identifier="+identifier, "error="+err.Error())
			return fmt.Errorf("删除备份失败: %w", err)
		}

		logging.Info("删除成功")
		logging.AuditLog("delete", dbType, "success", "identifier="+identifier)
		return nil
	})
}

// ValidateBackup 验证指定备份的有效性。
func (m *ManagerApp) ValidateBackup(ctx context.Context, dbType, validateID, backupType string) error {
	logging.InfoCtx(ctx, "验证备份", "id", validateID)

	backupTarget := filepath.Join(m.cfg.BaseBackupDir, dbType, "backup")

	backupTypeVal, err := backup.ParseBackupType(backupType)
	if err != nil {
		return fmt.Errorf("无效的备份类型: %s", backupType)
	}

	backupOpts := backup.BackupOptions{
		TargetPath: backupTarget,
		Type:       backupTypeVal,
	}

	return withDatabaseBackup(ctx, m.cfg, "validate", dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		if err := db.ValidateBackup(ctx, validateID, backupOpts); err != nil {
			logging.AuditLog("validate", dbType, "failed", "id="+validateID, "error="+err.Error())
			return fmt.Errorf("验证失败: %w", err)
		}

		logging.Info("备份有效")
		logging.AuditLog("validate", dbType, "success", "id="+validateID)
		return nil
	})
}

// GetBackupInfo 获取指定备份的详细信息。
func (m *ManagerApp) GetBackupInfo(ctx context.Context, dbType, infoID string) error {
	logging.InfoCtx(ctx, "获取备份信息", "id", infoID)

	backupTarget := filepath.Join(m.cfg.BaseBackupDir, dbType, "backup")
	backupOpts := backup.BackupOptions{TargetPath: backupTarget}

	return withDatabaseBackup(ctx, m.cfg, "info", dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		info, err := db.GetBackupInfo(ctx, infoID, backupOpts)
		if err != nil {
			return fmt.Errorf("获取备份信息失败: %w", err)
		}

		for key, value := range info {
			logging.InfoCtx(ctx, "备份信息", "key", key, "value", value)
		}

		return nil
	})
}

// RegisterBackup 将指定路径的备份注册到目录库。
func (m *ManagerApp) RegisterBackup(ctx context.Context, dbType, registerPath string) error {
	logging.InfoCtx(ctx, "注册备份", "path", registerPath)

	return withDatabaseBackup(ctx, m.cfg, "register", dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		if err := db.RegisterBackup(ctx, registerPath); err != nil {
			logging.AuditLog("register", dbType, "failed", "path="+registerPath, "error="+err.Error())
			return fmt.Errorf("注册备份失败: %w", err)
		}

		logging.Info("注册成功")
		logging.AuditLog("register", dbType, "success", "path="+registerPath)
		return nil
	})
}

// UnregisterBackup 从目录库中移除指定备份记录。
func (m *ManagerApp) UnregisterBackup(ctx context.Context, dbType, unregisterID string) error {
	logging.InfoCtx(ctx, "移除备份记录", "id", unregisterID)

	return withDatabaseBackup(ctx, m.cfg, "unregister", dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		if err := db.UnregisterBackup(ctx, unregisterID); err != nil {
			logging.AuditLog("unregister", dbType, "failed", "id="+unregisterID, "error="+err.Error())
			return fmt.Errorf("移除备份记录失败: %w", err)
		}

		logging.Info("移除成功")
		logging.AuditLog("unregister", dbType, "success", "id="+unregisterID)
		return nil
	})
}

// VerifyBackupStatus 检查备份状态并更新目录库。
func (m *ManagerApp) VerifyBackupStatus(ctx context.Context, dbType string) error {
	logging.Info("=== 检查备份状态 ===")

	return withDatabaseBackup(ctx, m.cfg, "verify_status", dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		if err := db.VerifyBackupStatus(ctx); err != nil {
			return fmt.Errorf("检查备份状态失败: %w", err)
		}

		logging.Info("检查完成")
		return nil
	})
}

// DeleteInvalidBackups 删除目录库中的无效备份记录。
func (m *ManagerApp) DeleteInvalidBackups(ctx context.Context, dbType string) error {
	logging.Info("=== 删除无效备份 ===")

	backupTarget := filepath.Join(m.cfg.BaseBackupDir, dbType, "backup")
	backupOpts := backup.BackupOptions{TargetPath: backupTarget}

	return withDatabaseBackup(ctx, m.cfg, "delete_invalid", dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		if err := db.DeleteInvalidBackups(ctx, backupOpts); err != nil {
			logging.AuditLog("delete_invalid", dbType, "failed", "error="+err.Error())
			return fmt.Errorf("删除无效备份失败: %w", err)
		}

		logging.Info("删除成功")
		logging.AuditLog("delete_invalid", dbType, "success")
		return nil
	})
}

// DeleteAllBackups 删除指定数据库的所有备份。
func (m *ManagerApp) DeleteAllBackups(ctx context.Context, dbType string) error {
	logging.Info("=== 删除所有备份 ===")

	if !confirmAction("确定要删除所有备份吗？此操作无法恢复！") {
		logging.Info("操作已取消")
		return nil
	}

	backupTarget := filepath.Join(m.cfg.BaseBackupDir, dbType, "backup")
	backupOpts := backup.BackupOptions{TargetPath: backupTarget}

	return withDatabaseBackup(ctx, m.cfg, "delete_all", dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		if err := db.DeleteAllBackups(ctx, backupOpts); err != nil {
			logging.AuditLog("delete_all", dbType, "failed", "error="+err.Error())
			return fmt.Errorf("删除所有备份失败: %w", err)
		}

		logging.Info("删除成功")
		logging.AuditLog("delete_all", dbType, "success")
		return nil
	})
}

// ValidateConfig 验证配置文件的有效性。
func (m *ManagerApp) ValidateConfig(configFilePath string) error {
	logging.Info("=== 验证配置文件 ===")

	if configFilePath == "" {
		return fmt.Errorf("必须指定配置文件路径")
	}

	logging.Info("正在验证配置文件", "path", configFilePath)

	if m.cfg.BaseBackupDir == "" {
		logging.Warn("警告: base_backup_dir 未配置，将使用默认路径")
	} else {
		logging.Info("备份基础目录", "dir", m.cfg.BaseBackupDir)
	}

	if len(m.cfg.Databases) == 0 {
		return fmt.Errorf("配置文件中没有定义任何数据库")
	}

	logging.Info("已配置的数据库数量", "count", len(m.cfg.Databases))

	for dbTypeKey, dbCfg := range m.cfg.Databases {
		logging.Info("验证数据库配置", "db_type", dbTypeKey)

		if dbCfg.Host == "" {
			logging.Warn("  警告: host 未配置")
		} else {
			logging.Info("主机", "host", dbCfg.Host)
		}

		if dbCfg.Port == 0 {
			logging.Warn("  警告: port 未配置，将使用默认端口")
		} else {
			logging.Info("端口", "port", dbCfg.Port)
		}

		if dbCfg.User == "" {
			return fmt.Errorf("数据库 %s 的 user 未配置", dbTypeKey)
		}
		logging.Info("用户", "user", dbCfg.User)

		if dbCfg.Password != "" {
			logging.Info("  密码: *** (已配置)")
		} else {
			logging.Warn("  警告: password 未配置")
		}

		if dbCfg.Database == "" {
			logging.Warn("  警告: database 未配置")
		} else {
			logging.Info("数据库", "database", dbCfg.Database)
		}

		if err := backup.ValidateDriverConfig(&dbCfg); err != nil {
			return fmt.Errorf("数据库 %s 的配置验证失败: %w", dbTypeKey, err)
		}
		logging.Info("  ✓ 配置验证通过")
	}

	logging.Info("\n=== 配置文件验证通过 ===")
	return nil
}

// FormatFileSize 格式化文件大小为人类可读字符串。
func FormatFileSize(size int64) string {
	return fileutil.FormatFileSize(size)
}

// confirmAction 请求用户确认操作。
func confirmAction(message string) bool {
	reader := bufio.NewReader(os.Stdin)
	logging.Warn(message)

	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

// logBackupInfo 输出备份信息到日志。
func logBackupInfo(b backup.BackupInfo) {
	var output string
	output = fmt.Sprintf("ID=%s, 完成时间=%s",
		b.BackupID, b.CompletionTime.Format("2006-01-02T15:04:05"))
	if b.BackupType != "" {
		output += fmt.Sprintf(", 类型=%s", b.BackupType)
	}
	if b.Size > 0 {
		output += fmt.Sprintf(", 大小=%s", FormatFileSize(b.Size))
	}
	if b.Status != "" {
		output += fmt.Sprintf(", 状态=%s", b.Status)
	}
	if b.Tag != "" {
		output += fmt.Sprintf(", 标签=%s", b.Tag)
	}
	if b.DeviceType != "" {
		output += fmt.Sprintf(", 设备类型=%s", b.DeviceType)
	}
	if b.BackupPath != "" {
		output += fmt.Sprintf(", 路径=%s", b.BackupPath)
	}
	logging.Info(output)
}
