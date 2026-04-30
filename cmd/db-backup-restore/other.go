package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"db-backup-restore/internal/backup"
	"db-backup-restore/pkg/utils"
)

var listDriversCmd = &cobra.Command{
	Use:   "list-drivers",
	Short: "列出所有支持的数据库驱动",
	Long: `列出所有已注册的数据库驱动及其支持的功能。

此命令可以帮助您了解当前工具支持哪些数据库类型，以及每种数据库支持哪些操作。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runListDrivers()
	},
}

var (
	deleteIdentifier string
	validateID       string
	infoID           string
	registerPath     string
	unregisterID     string
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "列出所有备份",
	Long: `列出数据库的所有备份。

使用示例:
  # 列出 MySQL 的所有备份
  db-backup-restore list -c config.json -t mysql

  # 列出 PostgreSQL 的所有备份
  db-backup-restore list -c config.json -t postgresql`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runListBackups(cmd.Context())
	},
}

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "删除指定备份",
	Long: `根据标识符删除指定备份。

使用示例:
  # 删除指定备份文件（MySQL）
  db-backup-restore delete -c config.json -t mysql --delete-identifier backup.sql

  # 删除指定备份（Oracle 使用备份集ID）
  db-backup-restore delete -c config.json -t oracle --delete-identifier "BS_12345"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDeleteBackup(cmd.Context())
	},
}

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "验证备份有效性",
	Long: `验证指定备份文件的完整性。

注意: 仅支持 Oracle/MSSQL 数据库。

使用示例:
  # 验证 Oracle 备份
  db-backup-restore validate -c config.json -t oracle --validate-id "BS_12345"

  # 验证 MSSQL 备份
  db-backup-restore validate -c config.json -t mssql --validate-id backup.bak`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runValidateBackup(cmd.Context())
	},
}

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "获取备份信息",
	Long: `获取指定备份的详细信息。

使用示例:
  # 获取 MySQL 备份信息
  db-backup-restore info -c config.json -t mysql --info-id backup.sql

  # 获取 Oracle 备份信息
  db-backup-restore info -c config.json -t oracle --info-id "BS_12345"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runGetBackupInfo(cmd.Context())
	},
}

var registerCmd = &cobra.Command{
	Use:   "register",
	Short: "注册备份到目录库",
	Long: `将指定路径的备份文件注册到备份目录库。

注意: 此命令仅 Oracle/MSSQL 支持。

使用示例:
  # 注册 Oracle 备份到目录库
  db-backup-restore register -c config.json -t oracle --register-path /backup/ORCL_backup_20240115`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRegisterBackup(cmd.Context())
	},
}

var unregisterCmd = &cobra.Command{
	Use:   "unregister",
	Short: "取消注册备份",
	Long: `从备份目录库中移除指定备份记录。

注意: 此命令仅 Oracle/MSSQL 支持。

使用示例:
  # 从 Oracle 目录库移除备份记录
  db-backup-restore unregister -c config.json -t oracle --unregister-id "BS_12345"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUnregisterBackup(cmd.Context())
	},
}

var verifyStatusCmd = &cobra.Command{
	Use:   "verify-status",
	Short: "验证备份状态",
	Long: `检查备份文件的状态并更新备份目录库。

注意: 此命令仅 Oracle/MSSQL 支持。

使用示例:
  # 验证 Oracle 备份状态
  db-backup-restore verify-status -c config.json -t oracle`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runVerifyBackupStatus(cmd.Context())
	},
}

var deleteInvalidCmd = &cobra.Command{
	Use:   "delete-invalid",
	Short: "删除无效备份",
	Long: `删除备份目录库中的无效备份记录。

注意: 此命令仅 Oracle/MSSQL 支持。

使用示例:
  # 删除 Oracle 无效备份
  db-backup-restore delete-invalid -c config.json -t oracle`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDeleteInvalidBackups(cmd.Context())
	},
}

var deleteAllCmd = &cobra.Command{
	Use:   "delete-all",
	Short: "删除所有备份",
	Long: `删除数据库的所有备份。

此操作将删除所有备份文件，且无法恢复。执行前需要确认。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDeleteAllBackups(cmd.Context())
	},
}

func init() {
	deleteCmd.Flags().StringVar(&deleteIdentifier, "delete-identifier", "", "删除备份的标识符")
	deleteCmd.MarkFlagRequired("delete-identifier")

	validateCmd.Flags().StringVar(&validateID, "validate-id", "", "验证备份的ID")
	validateCmd.MarkFlagRequired("validate-id")

	infoCmd.Flags().StringVar(&infoID, "info-id", "", "获取备份信息的ID")
	infoCmd.MarkFlagRequired("info-id")

	registerCmd.Flags().StringVar(&registerPath, "register-path", "", "注册备份的路径")
	registerCmd.MarkFlagRequired("register-path")

	unregisterCmd.Flags().StringVar(&unregisterID, "unregister-id", "", "移除备份记录的ID")
	unregisterCmd.MarkFlagRequired("unregister-id")

	rootCmd.AddCommand(listDriversCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(registerCmd)
	rootCmd.AddCommand(unregisterCmd)
	rootCmd.AddCommand(verifyStatusCmd)
	rootCmd.AddCommand(deleteInvalidCmd)
	rootCmd.AddCommand(deleteAllCmd)
}

func runListDrivers() error {
	utils.Info("=== 支持的数据库驱动 ===")

	drivers := backup.ListDriverMetadata()
	if len(drivers) == 0 {
		utils.Info("未注册任何数据库驱动")
		return nil
	}

	for _, driver := range drivers {
		utils.Infof("\n驱动名称: %s", driver.Name)
		utils.Infof("  版本: %s", driver.Version)
		utils.Infof("  描述: %s", driver.Description)
		utils.Infof("  支持的操作: %s", strings.Join(driver.SupportedActions, ", "))
		backupTypes := make([]string, 0, len(driver.SupportedBackupTypes))
		for _, bt := range driver.SupportedBackupTypes {
			backupTypes = append(backupTypes, string(bt))
		}
		utils.Infof("  支持的备份类型: %s", strings.Join(backupTypes, ", "))
	}

	return nil
}

func runListBackups(ctx context.Context) error {
	utils.Info("=== 列出所有备份 ===")

	dbCfg, err := appConfig.GetDBConfig(databaseType)
	if err != nil {
		return fmt.Errorf("获取数据库配置失败: %w", err)
	}

	db, err := backup.NewBackup(dbCfg)
	if err != nil {
		return fmt.Errorf("创建数据库备份实例失败: %w", err)
	}
	defer db.Close()

	backupTarget := filepath.Join(appConfig.BaseBackupDir, databaseType, "backup")
	backupOpts := backup.BackupOptions{TargetPath: backupTarget}

	backups, err := db.ListBackups(ctx, backupOpts)
	if err != nil {
		return fmt.Errorf("列出备份失败: %w", err)
	}

	if len(backups) == 0 {
		utils.Info("未找到备份")
		return nil
	}

	for _, b := range backups {
		logBackupInfo(b)
	}

	return nil
}

func runDeleteBackup(ctx context.Context) error {
	utils.Infof("=== 删除备份: %s ===", deleteIdentifier)

	dbCfg, err := appConfig.GetDBConfig(databaseType)
	if err != nil {
		return fmt.Errorf("获取数据库配置失败: %w", err)
	}

	db, err := backup.NewBackup(dbCfg)
	if err != nil {
		return fmt.Errorf("创建数据库备份实例失败: %w", err)
	}
	defer db.Close()

	backupTarget := filepath.Join(appConfig.BaseBackupDir, databaseType, "backup")
	backupOpts := backup.BackupOptions{TargetPath: backupTarget}

	if err := db.DeleteBackup(ctx, deleteIdentifier, backupOpts); err != nil {
		utils.AuditLog("delete", databaseType, "failed", "identifier="+deleteIdentifier, "error="+err.Error())
		return fmt.Errorf("删除备份失败: %w", err)
	}

	utils.Info("删除成功")
	utils.AuditLog("delete", databaseType, "success", "identifier="+deleteIdentifier)
	return nil
}

func runValidateBackup(ctx context.Context) error {
	utils.Infof("=== 验证备份: %s ===", validateID)

	dbCfg, err := appConfig.GetDBConfig(databaseType)
	if err != nil {
		return fmt.Errorf("获取数据库配置失败: %w", err)
	}

	db, err := backup.NewBackup(dbCfg)
	if err != nil {
		return fmt.Errorf("创建数据库备份实例失败: %w", err)
	}
	defer db.Close()

	backupTarget := filepath.Join(appConfig.BaseBackupDir, databaseType, "backup")

	backupTypeVal, err := backup.ParseBackupType(backupType)
	if err != nil {
		return fmt.Errorf("无效的备份类型: %s", backupType)
	}

	backupOpts := backup.BackupOptions{
		TargetPath: backupTarget,
		Type:       backupTypeVal,
	}

	if err := db.ValidateBackup(ctx, validateID, backupOpts); err != nil {
		utils.AuditLog("validate", databaseType, "failed", "id="+validateID, "error="+err.Error())
		return fmt.Errorf("验证失败: %w", err)
	}

	utils.Info("备份有效")
	utils.AuditLog("validate", databaseType, "success", "id="+validateID)
	return nil
}

func runGetBackupInfo(ctx context.Context) error {
	utils.Infof("=== 获取备份信息: %s ===", infoID)

	dbCfg, err := appConfig.GetDBConfig(databaseType)
	if err != nil {
		return fmt.Errorf("获取数据库配置失败: %w", err)
	}

	db, err := backup.NewBackup(dbCfg)
	if err != nil {
		return fmt.Errorf("创建数据库备份实例失败: %w", err)
	}
	defer db.Close()

	backupTarget := filepath.Join(appConfig.BaseBackupDir, databaseType, "backup")
	backupOpts := backup.BackupOptions{TargetPath: backupTarget}

	info, err := db.GetBackupInfo(ctx, infoID, backupOpts)
	if err != nil {
		return fmt.Errorf("获取备份信息失败: %w", err)
	}

	for key, value := range info {
		utils.Infof("  %s: %s", key, value)
	}

	return nil
}

func runRegisterBackup(ctx context.Context) error {
	utils.Infof("=== 注册备份: %s ===", registerPath)

	dbCfg, err := appConfig.GetDBConfig(databaseType)
	if err != nil {
		return fmt.Errorf("获取数据库配置失败: %w", err)
	}

	db, err := backup.NewBackup(dbCfg)
	if err != nil {
		return fmt.Errorf("创建数据库备份实例失败: %w", err)
	}
	defer db.Close()

	if err := db.RegisterBackup(ctx, registerPath); err != nil {
		utils.AuditLog("register", databaseType, "failed", "path="+registerPath, "error="+err.Error())
		return fmt.Errorf("注册备份失败: %w", err)
	}

	utils.Info("注册成功")
	utils.AuditLog("register", databaseType, "success", "path="+registerPath)
	return nil
}

func runUnregisterBackup(ctx context.Context) error {
	utils.Infof("=== 移除备份记录: %s ===", unregisterID)

	dbCfg, err := appConfig.GetDBConfig(databaseType)
	if err != nil {
		return fmt.Errorf("获取数据库配置失败: %w", err)
	}

	db, err := backup.NewBackup(dbCfg)
	if err != nil {
		return fmt.Errorf("创建数据库备份实例失败: %w", err)
	}
	defer db.Close()

	if err := db.UnregisterBackup(ctx, unregisterID); err != nil {
		utils.AuditLog("unregister", databaseType, "failed", "id="+unregisterID, "error="+err.Error())
		return fmt.Errorf("移除备份记录失败: %w", err)
	}

	utils.Info("移除成功")
	utils.AuditLog("unregister", databaseType, "success", "id="+unregisterID)
	return nil
}

func runVerifyBackupStatus(ctx context.Context) error {
	utils.Info("=== 检查备份状态 ===")

	dbCfg, err := appConfig.GetDBConfig(databaseType)
	if err != nil {
		return fmt.Errorf("获取数据库配置失败: %w", err)
	}

	db, err := backup.NewBackup(dbCfg)
	if err != nil {
		return fmt.Errorf("创建数据库备份实例失败: %w", err)
	}
	defer db.Close()

	if err := db.VerifyBackupStatus(ctx); err != nil {
		return fmt.Errorf("检查备份状态失败: %w", err)
	}

	utils.Info("检查完成")
	return nil
}

func runDeleteInvalidBackups(ctx context.Context) error {
	utils.Info("=== 删除无效备份 ===")

	dbCfg, err := appConfig.GetDBConfig(databaseType)
	if err != nil {
		return fmt.Errorf("获取数据库配置失败: %w", err)
	}

	db, err := backup.NewBackup(dbCfg)
	if err != nil {
		return fmt.Errorf("创建数据库备份实例失败: %w", err)
	}
	defer db.Close()

	backupTarget := filepath.Join(appConfig.BaseBackupDir, databaseType, "backup")
	backupOpts := backup.BackupOptions{TargetPath: backupTarget}

	if err := db.DeleteInvalidBackups(ctx, backupOpts); err != nil {
		utils.AuditLog("delete_invalid", databaseType, "failed", "error="+err.Error())
		return fmt.Errorf("删除无效备份失败: %w", err)
	}

	utils.Info("删除成功")
	utils.AuditLog("delete_invalid", databaseType, "success")
	return nil
}

func runDeleteAllBackups(ctx context.Context) error {
	utils.Info("=== 删除所有备份 ===")

	if !confirmAction("确定要删除所有备份吗？此操作无法恢复！") {
		utils.Info("操作已取消")
		return nil
	}

	dbCfg, err := appConfig.GetDBConfig(databaseType)
	if err != nil {
		return fmt.Errorf("获取数据库配置失败: %w", err)
	}

	db, err := backup.NewBackup(dbCfg)
	if err != nil {
		return fmt.Errorf("创建数据库备份实例失败: %w", err)
	}
	defer db.Close()

	backupTarget := filepath.Join(appConfig.BaseBackupDir, databaseType, "backup")
	backupOpts := backup.BackupOptions{TargetPath: backupTarget}

	if err := db.DeleteAllBackups(ctx, backupOpts); err != nil {
		utils.AuditLog("delete_all", databaseType, "failed", "error="+err.Error())
		return fmt.Errorf("删除所有备份失败: %w", err)
	}

	utils.Info("删除成功")
	utils.AuditLog("delete_all", databaseType, "success")
	return nil
}

func confirmAction(message string) bool {
	reader := bufio.NewReader(os.Stdin)
	utils.Warnf("%s (y/N): ", message)

	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

func logBackupInfo(b backup.BackupInfo) {
	var output string
	output = fmt.Sprintf("ID=%s, 完成时间=%s",
		b.BackupID, b.CompletionTime.Format("2006-01-02T15:04:05"))
	if b.BackupType != "" {
		output += fmt.Sprintf(", 类型=%s", b.BackupType)
	}
	if b.Size > 0 {
		output += fmt.Sprintf(", 大小=%s", utils.FormatFileSize(b.Size))
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
	utils.Info(output)
}
