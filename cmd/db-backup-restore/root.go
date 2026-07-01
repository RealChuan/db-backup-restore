package main

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/spf13/cobra"

	"github.com/RealChuan/db-backup-restore/internal/app"
	"github.com/RealChuan/db-backup-restore/internal/backup"
	"github.com/RealChuan/db-backup-restore/internal/config"
	"github.com/RealChuan/db-backup-restore/internal/logging"
)

var (
	configFilePath string
	databaseType   string
	backupType     string
	outputFormat   string
	notifyWebhook  string
	appConfig      *config.Config

	loggerInitOnce sync.Once
)

var validateConfigCmd = &cobra.Command{
	Use:   "validate-config",
	Short: "验证配置文件的有效性",
	Long: `验证配置文件的语法和内容是否有效。

此命令可以在执行备份/还原操作之前验证配置文件，避免运行时错误。`,
	RunE: func(_ *cobra.Command, _ []string) error {
		return runValidateConfig()
	},
}

var rootCmd = &cobra.Command{
	Use:   "db-backup-restore",
	Short: "数据库备份/还原工具",
	Long: `db-backup-restore 是一个命令行工具，用于备份和还原多种数据库。
支持的数据库类型: MySQL, PostgreSQL, Oracle, MSSQL

使用示例:
  db-backup-restore backup -c config.json -t mysql
  db-backup-restore restore -c config.json -t mysql --backup-identifier /path/to/backup.sql
  db-backup-restore list-drivers
  db-backup-restore validate-config -c config.json`,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		var initErr error
		loggerInitOnce.Do(func() {
			if configFilePath != "" {
				var err error
				appConfig, err = config.LoadConfig(configFilePath)
				if err != nil {
					initErr = fmt.Errorf("加载配置文件失败: %w", err)
					return
				}
				logConfig := appConfig.GetLogConfig()
				if err := logging.Init(logConfig); err != nil {
					initErr = fmt.Errorf("初始化日志系统失败: %w", err)
					return
				}
			} else {
				if err := logging.Init(logging.DefaultConfig()); err != nil {
					initErr = fmt.Errorf("初始化日志系统失败: %w", err)
					return
				}
			}
		})
		if initErr != nil {
			return initErr
		}

		if cmd.Name() == "validate-config" || cmd.Name() == "list-drivers" || cmd.Name() == "help" {
			return nil
		}

		if configFilePath == "" {
			return errors.New("必须指定配置文件路径: -c/--config")
		}

		return nil
	},
}

func Execute() {
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	SetupCommandErrorHandling(rootCmd)

	if err := rootCmd.Execute(); err != nil {
		// PersistentPreRunE 错误（如缺少 -c）在 RunE 之前发生，
		// outputResult 未被调用，需在此兜底输出。
		writer := app.NewOutputWriter(currentFormat())
		_ = writer.Write(&app.OperationResult{
			Success: false,
			Error:   err.Error(),
		})
		os.Exit(1)
	}
}

func runValidateConfig() error {
	if configFilePath == "" {
		return errors.New("必须通过 -c/--config 参数指定配置文件路径")
	}

	cfg, err := config.LoadConfig(configFilePath)
	if err != nil {
		return fmt.Errorf("配置文件加载失败: %w", err)
	}

	result, err := app.NewManagerApp(cfg).ValidateConfig(configFilePath)
	return outputResult(result, err, "validate_config")
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&configFilePath, "config", "c", "", "配置文件路径")
	rootCmd.PersistentFlags().StringVarP(&databaseType, "db-type", "t", "mysql", "数据库类型: mysql, postgresql, oracle, mssql")
	rootCmd.PersistentFlags().StringVar(&backupType, "backup-type", "logical", "备份类型: logical(逻辑备份/SQL文件), physical(物理备份/数据文件)")
	rootCmd.PersistentFlags().StringVar(&outputFormat, "output", "text", "输出格式: text, json")
	rootCmd.PersistentFlags().StringVar(&notifyWebhook, "notify", "", "操作失败时发送 webhook 通知的 URL")

	rootCmd.AddCommand(validateConfigCmd)
}

// currentFormat 解析当前 --output 标志为 OutputFormat。
func currentFormat() backup.OutputFormat {
	f, _ := backup.ParseOutputFormat(outputFormat)
	return f
}

// outputResult 统一渲染命令结果。成功时写入 result，失败时写入错误 result。
// 返回 error 供 cobra 设置退出码。
func outputResult(result *app.OperationResult, err error, operation string) error {
	writer := app.NewOutputWriter(currentFormat())
	if err != nil {
		_ = writer.Write(&app.OperationResult{
			Success:   false,
			Operation: operation,
			DBType:    databaseType,
			Error:     err.Error(),
		})
		return err
	}
	if result != nil {
		return writer.Write(result)
	}
	return nil
}
