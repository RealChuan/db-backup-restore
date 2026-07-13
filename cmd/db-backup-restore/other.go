package main

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/RealChuan/db-backup-restore/internal/app"
	"github.com/RealChuan/db-backup-restore/internal/backup"
)

var listDriversCmd = &cobra.Command{
	Use:   "list-drivers",
	Short: "列出所有支持的数据库驱动",
	Long: `列出所有已注册的数据库驱动及其支持的功能。

此命令可以帮助您了解当前工具支持哪些数据库类型，以及每种数据库支持哪些操作。`,
	RunE: func(_ *cobra.Command, _ []string) error {
		return runListDrivers()
	},
}

var (
	deleteIdentifier string
	validateID       string
	infoID           string
	registerPath     string
	unregisterID     string
	listBackupType   string
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
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runListBackups(cmd.Context())
	},
}

var listDatabasesCmd = &cobra.Command{
	Use:   "list-databases",
	Short: "列出所有用户数据库",
	Long: `列出指定数据库类型下的所有用户数据库（排除系统数据库）。

注意: Oracle 数据库基于实例架构，不支持此操作。

使用示例:
  # 列出 MySQL 的所有用户数据库（text 格式）
  db-backup-restore list-databases -c config.json -t mysql

  # 以 JSON 格式输出（建议在配置文件中设置 log.level=error 以保证输出为纯 JSON）
  db-backup-restore list-databases -c config.json -t postgresql --output json`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runListDatabases(cmd.Context())
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
	RunE: func(cmd *cobra.Command, _ []string) error {
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
	RunE: func(cmd *cobra.Command, _ []string) error {
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
	RunE: func(cmd *cobra.Command, _ []string) error {
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
	RunE: func(cmd *cobra.Command, _ []string) error {
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
	RunE: func(cmd *cobra.Command, _ []string) error {
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
	RunE: func(cmd *cobra.Command, _ []string) error {
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
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runDeleteInvalidBackups(cmd.Context())
	},
}

var deleteAllCmd = &cobra.Command{
	Use:   "delete-all",
	Short: "删除所有备份",
	Long: `删除数据库的所有备份。

此操作将删除所有备份文件，且无法恢复。执行前需要确认。`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runDeleteAllBackups(cmd.Context())
	},
}

var archiveDest string

var enableArchiveCmd = &cobra.Command{
	Use:   "enable-archive",
	Short: "启用归档模式",
	Long: `启用数据库的归档模式（ARCHIVELOG）。

注意: 仅 Oracle 和达梦支持此操作。开启归档模式是执行联机物理备份的前提。

使用示例:
  # 启用达梦归档模式（指定归档目录）
  db-backup-restore enable-archive -c config.json -t dameng --archive-dest c:/work/database_backup/dameng/physical/archivelog

  # 启用 Oracle 归档模式（使用默认归档目录）
  db-backup-restore enable-archive -c config.json -t oracle`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runEnableArchive(cmd.Context())
	},
}

var disableArchiveCmd = &cobra.Command{
	Use:   "disable-archive",
	Short: "关闭归档模式",
	Long: `关闭数据库的归档模式（切换为 NOARCHIVELOG）。

注意: 仅 Oracle 和达梦支持此操作。关闭归档模式后将无法执行联机物理备份。

使用示例:
  # 关闭达梦归档模式
  db-backup-restore disable-archive -c config.json -t dameng

  # 关闭 Oracle 归档模式
  db-backup-restore disable-archive -c config.json -t oracle`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runDisableArchive(cmd.Context())
	},
}

func init() {
	listCmd.Flags().StringVar(&listBackupType, "backup-type", "", "备份类型筛选: logical, physical（为空则列出所有类型）")

	deleteCmd.Flags().StringVar(&deleteIdentifier, "delete-identifier", "", "删除备份的标识符")
	if err := deleteCmd.MarkFlagRequired("delete-identifier"); err != nil {
		log.Fatal(err)
	}

	validateCmd.Flags().StringVar(&validateID, "validate-id", "", "验证备份的ID")
	if err := validateCmd.MarkFlagRequired("validate-id"); err != nil {
		log.Fatal(err)
	}

	infoCmd.Flags().StringVar(&infoID, "info-id", "", "获取备份信息的ID")
	if err := infoCmd.MarkFlagRequired("info-id"); err != nil {
		log.Fatal(err)
	}

	registerCmd.Flags().StringVar(&registerPath, "register-path", "", "注册备份的路径")
	if err := registerCmd.MarkFlagRequired("register-path"); err != nil {
		log.Fatal(err)
	}

	unregisterCmd.Flags().StringVar(&unregisterID, "unregister-id", "", "移除备份记录的ID")
	if err := unregisterCmd.MarkFlagRequired("unregister-id"); err != nil {
		log.Fatal(err)
	}

	rootCmd.AddCommand(listDriversCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(listDatabasesCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(registerCmd)
	rootCmd.AddCommand(unregisterCmd)
	rootCmd.AddCommand(verifyStatusCmd)
	rootCmd.AddCommand(deleteInvalidCmd)
	rootCmd.AddCommand(deleteAllCmd)

	enableArchiveCmd.Flags().StringVar(&archiveDest, "archive-dest", "", "归档日志存储目录路径（为空则使用数据库默认配置）")
	rootCmd.AddCommand(enableArchiveCmd)
	rootCmd.AddCommand(disableArchiveCmd)
}

func runListDrivers() error {
	drivers := backup.ListDriverMetadata()

	items := make([]interface{}, 0, len(drivers))
	for _, driver := range drivers {
		backupTypes := make([]interface{}, 0, len(driver.SupportedBackupTypes))
		for _, bt := range driver.SupportedBackupTypes {
			backupTypes = append(backupTypes, string(bt))
		}
		actions := make([]interface{}, 0, len(driver.SupportedActions))
		for _, a := range driver.SupportedActions {
			actions = append(actions, a)
		}
		items = append(items, map[string]interface{}{
			"name":                   driver.Name,
			"version":                driver.Version,
			"description":            driver.Description,
			"supported_actions":      actions,
			"supported_backup_types": backupTypes,
		})
	}

	result := &app.OperationResult{
		Success:   true,
		Operation: app.OpListDrivers,
		Message:   fmt.Sprintf("共 %d 个驱动", len(drivers)),
		Data:      map[string]interface{}{"drivers": items},
	}
	return app.NewOutputWriter(currentFormat()).Write(result)
}

func runListBackups(ctx context.Context) error {
	result, err := app.NewManagerApp(appConfig).ListBackups(ctx, databaseType, listBackupType)
	return outputResult(result, err, "list")
}

func runListDatabases(ctx context.Context) error {
	result, err := app.NewManagerApp(appConfig).ListDatabases(ctx, databaseType)
	return outputResult(result, err, "list_databases")
}

func runDeleteBackup(ctx context.Context) error {
	result, err := app.NewManagerApp(appConfig).DeleteBackup(ctx, databaseType, deleteIdentifier)
	return outputResult(result, err, "delete")
}

func runValidateBackup(ctx context.Context) error {
	result, err := app.NewManagerApp(appConfig).ValidateBackup(ctx, databaseType, validateID, backupType)
	return outputResult(result, err, "validate")
}

func runGetBackupInfo(ctx context.Context) error {
	result, err := app.NewManagerApp(appConfig).GetBackupInfo(ctx, databaseType, infoID)
	return outputResult(result, err, "info")
}

func runRegisterBackup(ctx context.Context) error {
	result, err := app.NewManagerApp(appConfig).RegisterBackup(ctx, databaseType, registerPath)
	return outputResult(result, err, "register")
}

func runUnregisterBackup(ctx context.Context) error {
	result, err := app.NewManagerApp(appConfig).UnregisterBackup(ctx, databaseType, unregisterID)
	return outputResult(result, err, "unregister")
}

func runVerifyBackupStatus(ctx context.Context) error {
	result, err := app.NewManagerApp(appConfig).VerifyBackupStatus(ctx, databaseType)
	return outputResult(result, err, "verify_status")
}

func runDeleteInvalidBackups(ctx context.Context) error {
	result, err := app.NewManagerApp(appConfig).DeleteInvalidBackups(ctx, databaseType)
	return outputResult(result, err, "delete_invalid")
}

func runDeleteAllBackups(ctx context.Context) error {
	result, err := app.NewManagerApp(appConfig).DeleteAllBackups(ctx, databaseType)
	return outputResult(result, err, "delete_all")
}

func runEnableArchive(ctx context.Context) error {
	result, err := app.NewManagerApp(appConfig).EnableArchiveLog(ctx, databaseType, archiveDest)
	return outputResult(result, err, "enable_archive")
}

func runDisableArchive(ctx context.Context) error {
	result, err := app.NewManagerApp(appConfig).DisableArchiveLog(ctx, databaseType)
	return outputResult(result, err, "disable_archive")
}
