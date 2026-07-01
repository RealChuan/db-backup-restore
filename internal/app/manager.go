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
func (m *ManagerApp) ListBackups(ctx context.Context, dbType string) (*OperationResult, error) {
	logging.Debug("=== 列出所有备份 ===")

	backupTarget := filepath.Join(m.cfg.BaseBackupDir, dbType, "backup")
	backupOpts := backup.BackupOptions{TargetPath: backupTarget}

	var backups []backup.BackupInfo
	err := withDatabaseBackup(ctx, m.cfg, OpList, dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		var err error
		backups, err = db.ListBackups(ctx, backupOpts)
		if err != nil {
			return fmt.Errorf("列出备份失败: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	data := map[string]interface{}{}
	if len(backups) == 0 {
		return &OperationResult{
			Success:   true,
			Operation: OpList,
			DBType:    dbType,
			Message:   "未找到备份",
			Data:      data,
		}, nil
	}

	items := make([]interface{}, 0, len(backups))
	for _, b := range backups {
		items = append(items, backupInfoToMap(b))
	}
	data[DataKeyBackups] = items

	return &OperationResult{
		Success:   true,
		Operation: OpList,
		DBType:    dbType,
		Message:   fmt.Sprintf("共 %d 个备份", len(backups)),
		Data:      data,
	}, nil
}

// ListDatabases 列出指定数据库类型下的所有用户数据库。
func (m *ManagerApp) ListDatabases(ctx context.Context, dbType string) (*OperationResult, error) {
	logging.Debug("=== 列出所有数据库 ===")

	var databases []string
	err := withDatabaseBackup(ctx, m.cfg, OpListDatabases, dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		var err error
		databases, err = db.ListDatabases(ctx)
		if err != nil {
			return fmt.Errorf("列出数据库失败: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	items := make([]interface{}, 0, len(databases))
	for _, d := range databases {
		items = append(items, d)
	}

	return &OperationResult{
		Success:   true,
		Operation: OpListDatabases,
		DBType:    dbType,
		Message:   fmt.Sprintf("共 %d 个数据库", len(databases)),
		Data:      map[string]interface{}{DataKeyDatabases: items},
	}, nil
}

// DeleteBackup 删除指定标识符的备份。
func (m *ManagerApp) DeleteBackup(ctx context.Context, dbType, identifier string) (*OperationResult, error) {
	logging.DebugCtx(ctx, "删除备份", "identifier", identifier)

	backupTarget := filepath.Join(m.cfg.BaseBackupDir, dbType, "backup")
	backupOpts := backup.BackupOptions{TargetPath: backupTarget}

	err := withDatabaseBackup(ctx, m.cfg, OpDelete, dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		if err := db.DeleteBackup(ctx, identifier, backupOpts); err != nil {
			logging.AuditLog("delete", dbType, "failed", "identifier="+identifier, "error="+err.Error())
			return fmt.Errorf("删除备份失败: %w", err)
		}
		logging.AuditLog("delete", dbType, "success", "identifier="+identifier)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &OperationResult{
		Success:   true,
		Operation: OpDelete,
		DBType:    dbType,
		Message:   MsgDeleteSuccess,
		Data:      map[string]interface{}{DataKeyIdentifier: identifier},
	}, nil
}

// ValidateBackup 验证指定备份的有效性。
func (m *ManagerApp) ValidateBackup(ctx context.Context, dbType, validateID, backupType string) (*OperationResult, error) {
	logging.DebugCtx(ctx, "验证备份", "id", validateID)

	backupTarget := filepath.Join(m.cfg.BaseBackupDir, dbType, "backup")

	backupTypeVal, err := backup.ParseBackupType(backupType)
	if err != nil {
		return nil, fmt.Errorf("无效的备份类型: %s", backupType)
	}

	backupOpts := backup.BackupOptions{
		TargetPath: backupTarget,
		Type:       backupTypeVal,
	}

	err = withDatabaseBackup(ctx, m.cfg, OpValidate, dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		if err := db.ValidateBackup(ctx, validateID, backupOpts); err != nil {
			logging.AuditLog("validate", dbType, "failed", "id="+validateID, "error="+err.Error())
			return fmt.Errorf("验证失败: %w", err)
		}
		logging.AuditLog("validate", dbType, "success", "id="+validateID)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &OperationResult{
		Success:   true,
		Operation: OpValidate,
		DBType:    dbType,
		Message:   "验证通过",
		Data:      map[string]interface{}{DataKeyID: validateID},
	}, nil
}

// GetBackupInfo 获取指定备份的详细信息。
func (m *ManagerApp) GetBackupInfo(ctx context.Context, dbType, infoID string) (*OperationResult, error) {
	logging.DebugCtx(ctx, "获取备份信息", "id", infoID)

	backupTarget := filepath.Join(m.cfg.BaseBackupDir, dbType, "backup")
	backupOpts := backup.BackupOptions{TargetPath: backupTarget}

	var info map[string]string
	err := withDatabaseBackup(ctx, m.cfg, OpInfo, dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		var err error
		info, err = db.GetBackupInfo(ctx, infoID, backupOpts)
		if err != nil {
			return fmt.Errorf("获取备份信息失败: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	data := make(map[string]interface{}, len(info))
	for k, v := range info {
		data[k] = v
	}

	return &OperationResult{
		Success:   true,
		Operation: OpInfo,
		DBType:    dbType,
		Data:      data,
	}, nil
}

// RegisterBackup 将指定路径的备份注册到目录库。
func (m *ManagerApp) RegisterBackup(ctx context.Context, dbType, registerPath string) (*OperationResult, error) {
	logging.DebugCtx(ctx, "注册备份", "path", registerPath)

	err := withDatabaseBackup(ctx, m.cfg, OpRegister, dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		if err := db.RegisterBackup(ctx, registerPath); err != nil {
			logging.AuditLog("register", dbType, "failed", "path="+registerPath, "error="+err.Error())
			return fmt.Errorf("注册备份失败: %w", err)
		}
		logging.AuditLog("register", dbType, "success", "path="+registerPath)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &OperationResult{
		Success:   true,
		Operation: OpRegister,
		DBType:    dbType,
		Message:   "注册成功",
		Data:      map[string]interface{}{DataKeyPath: registerPath},
	}, nil
}

// UnregisterBackup 从目录库中移除指定备份记录。
func (m *ManagerApp) UnregisterBackup(ctx context.Context, dbType, unregisterID string) (*OperationResult, error) {
	logging.DebugCtx(ctx, "移除备份记录", "id", unregisterID)

	err := withDatabaseBackup(ctx, m.cfg, OpUnregister, dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		if err := db.UnregisterBackup(ctx, unregisterID); err != nil {
			logging.AuditLog("unregister", dbType, "failed", "id="+unregisterID, "error="+err.Error())
			return fmt.Errorf("移除备份记录失败: %w", err)
		}
		logging.AuditLog("unregister", dbType, "success", "id="+unregisterID)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &OperationResult{
		Success:   true,
		Operation: OpUnregister,
		DBType:    dbType,
		Message:   "移除成功",
		Data:      map[string]interface{}{DataKeyID: unregisterID},
	}, nil
}

// VerifyBackupStatus 检查备份状态并更新目录库。
func (m *ManagerApp) VerifyBackupStatus(ctx context.Context, dbType string) (*OperationResult, error) {
	logging.Debug("=== 检查备份状态 ===")

	err := withDatabaseBackup(ctx, m.cfg, OpVerifyStatus, dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		if err := db.VerifyBackupStatus(ctx); err != nil {
			return fmt.Errorf("检查备份状态失败: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &OperationResult{
		Success:   true,
		Operation: OpVerifyStatus,
		DBType:    dbType,
		Message:   "检查完成",
	}, nil
}

// DeleteInvalidBackups 删除目录库中的无效备份记录。
func (m *ManagerApp) DeleteInvalidBackups(ctx context.Context, dbType string) (*OperationResult, error) {
	logging.Debug("=== 删除无效备份 ===")

	backupTarget := filepath.Join(m.cfg.BaseBackupDir, dbType, "backup")
	backupOpts := backup.BackupOptions{TargetPath: backupTarget}

	err := withDatabaseBackup(ctx, m.cfg, OpDeleteInvalid, dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		if err := db.DeleteInvalidBackups(ctx, backupOpts); err != nil {
			logging.AuditLog("delete_invalid", dbType, "failed", "error="+err.Error())
			return fmt.Errorf("删除无效备份失败: %w", err)
		}
		logging.AuditLog("delete_invalid", dbType, "success")
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &OperationResult{
		Success:   true,
		Operation: OpDeleteInvalid,
		DBType:    dbType,
		Message:   MsgDeleteSuccess,
	}, nil
}

// DeleteAllBackups 删除指定数据库的所有备份。
func (m *ManagerApp) DeleteAllBackups(ctx context.Context, dbType string) (*OperationResult, error) {
	logging.Debug("=== 删除所有备份 ===")

	if !confirmAction("确定要删除所有备份吗？此操作无法恢复！") {
		return &OperationResult{
			Success:   true,
			Operation: OpDeleteAll,
			DBType:    dbType,
			Message:   "操作已取消",
		}, nil
	}

	backupTarget := filepath.Join(m.cfg.BaseBackupDir, dbType, "backup")
	backupOpts := backup.BackupOptions{TargetPath: backupTarget}

	err := withDatabaseBackup(ctx, m.cfg, OpDeleteAll, dbType, func(ctx context.Context, db backup.DatabaseBackup) error {
		if err := db.DeleteAllBackups(ctx, backupOpts); err != nil {
			logging.AuditLog("delete_all", dbType, "failed", "error="+err.Error())
			return fmt.Errorf("删除所有备份失败: %w", err)
		}
		logging.AuditLog("delete_all", dbType, "success")
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &OperationResult{
		Success:   true,
		Operation: OpDeleteAll,
		DBType:    dbType,
		Message:   MsgDeleteSuccess,
	}, nil
}

// ValidateConfig 验证配置文件的有效性。
func (m *ManagerApp) ValidateConfig(configFilePath string) (*OperationResult, error) {
	logging.Debug("=== 验证配置文件 ===")

	if configFilePath == "" {
		return nil, fmt.Errorf("必须指定配置文件路径")
	}

	logging.Debug("正在验证配置文件", "path", configFilePath)

	if m.cfg.BaseBackupDir == "" {
		logging.Warn("警告: base_backup_dir 未配置，将使用默认路径")
	} else {
		logging.Debug("备份基础目录", "dir", m.cfg.BaseBackupDir)
	}

	if len(m.cfg.Databases) == 0 {
		return nil, fmt.Errorf("配置文件中没有定义任何数据库")
	}

	logging.Debug("已配置的数据库数量", "count", len(m.cfg.Databases))

	for dbTypeKey, dbCfg := range m.cfg.Databases {
		logging.Debug("验证数据库配置", "db_type", dbTypeKey)

		if dbCfg.Host == "" {
			logging.Warn("  警告: host 未配置")
		}

		if dbCfg.Port == 0 {
			logging.Warn("  警告: port 未配置，将使用默认端口")
		}

		if dbCfg.User == "" {
			return nil, fmt.Errorf("数据库 %s 的 user 未配置", dbTypeKey)
		}

		if dbCfg.Password == "" {
			logging.Warn("  警告: password 未配置")
		}

		if dbCfg.Database == "" {
			logging.Warn("  警告: database 未配置")
		}

		if err := backup.ValidateDriverConfig(&dbCfg); err != nil {
			return nil, fmt.Errorf("数据库 %s 的配置验证失败: %w", dbTypeKey, err)
		}
	}

	return &OperationResult{
		Success:   true,
		Operation: OpValidateConfig,
		Message:   "配置验证通过",
		Data: map[string]interface{}{
			"databases_count": len(m.cfg.Databases),
			"base_backup_dir": m.cfg.BaseBackupDir,
		},
	}, nil
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

// backupInfoToMap 将 BackupInfo 转换为 map 用于结构化输出。
func backupInfoToMap(b backup.BackupInfo) map[string]interface{} {
	m := map[string]interface{}{
		"id":   b.BackupID,
		"time": b.CompletionTime.Format("2006-01-02T15:04:05"),
	}
	if b.BackupType != "" {
		m["type"] = b.BackupType
	}
	if b.Size > 0 {
		m[DataKeySize] = FormatFileSize(b.Size)
	}
	if b.Status != "" {
		m["status"] = b.Status
	}
	if b.Tag != "" {
		m["tag"] = b.Tag
	}
	if b.DeviceType != "" {
		m["device_type"] = b.DeviceType
	}
	if b.BackupPath != "" {
		m["path"] = b.BackupPath
	}
	return m
}
