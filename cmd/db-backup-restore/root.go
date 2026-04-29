package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"db-backup-restore/internal/backup"
	"db-backup-restore/internal/config"
	"db-backup-restore/pkg/utils"
)

var (
	configFilePath    string
	databaseType      string
	appConfig         *config.Config
	loggerInitialized bool
)

var validateConfigCmd = &cobra.Command{
	Use:   "validate-config",
	Short: "验证配置文件的有效性",
	Long: `验证配置文件的语法和内容是否有效。

此命令可以在执行备份/还原操作之前验证配置文件，避免运行时错误。`,
	RunE: func(cmd *cobra.Command, args []string) error {
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
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if !loggerInitialized {
			if configFilePath != "" {
				var err error
				appConfig, err = config.LoadConfig(configFilePath)
				if err != nil {
					return fmt.Errorf("加载配置文件失败: %w", err)
				}
				logConfig := appConfig.GetLogConfig()
				if err := utils.InitLogger(logConfig); err != nil {
					return fmt.Errorf("初始化日志系统失败: %w", err)
				}
			} else {
				if err := utils.InitLogger(utils.NewLogConfig()); err != nil {
					return fmt.Errorf("初始化日志系统失败: %w", err)
				}
			}
			utils.InitTraceID()
			loggerInitialized = true
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
	SetupCommandErrorHandling(rootCmd)

	err := rootCmd.Execute()
	if err != nil {
		backup.HandleError(err)
		utils.Fatal(err)
	}
}

func runValidateConfig() error {
	utils.Info("=== 验证配置文件 ===")

	if configFilePath == "" {
		return errors.New("必须通过 -c/--config 参数指定配置文件路径")
	}

	utils.Infof("正在验证配置文件: %s", configFilePath)

	cfg, err := config.LoadConfig(configFilePath)
	if err != nil {
		return fmt.Errorf("配置文件加载失败: %w", err)
	}

	if cfg.BaseBackupDir == "" {
		utils.Warn("警告: base_backup_dir 未配置，将使用默认路径")
	} else {
		utils.Infof("备份基础目录: %s", cfg.BaseBackupDir)
	}

	if len(cfg.Databases) == 0 {
		return errors.New("配置文件中没有定义任何数据库")
	}

	utils.Infof("已配置的数据库数量: %d", len(cfg.Databases))

	for dbTypeKey, dbCfg := range cfg.Databases {
		utils.Infof("\n验证数据库配置: %s", dbTypeKey)

		if dbCfg.Host == "" {
			utils.Warn("  警告: host 未配置")
		} else {
			utils.Infof("  主机: %s", dbCfg.Host)
		}

		if dbCfg.Port == 0 {
			utils.Warn("  警告: port 未配置，将使用默认端口")
		} else {
			utils.Infof("  端口: %d", dbCfg.Port)
		}

		if dbCfg.User == "" {
			return fmt.Errorf("数据库 %s 的 user 未配置", dbTypeKey)
		}
		utils.Infof("  用户: %s", dbCfg.User)

		if dbCfg.Password != "" {
			utils.Infof("  密码: *** (已配置)")
		} else {
			utils.Warn("  警告: password 未配置")
		}

		if dbCfg.Database == "" {
			utils.Warn("  警告: database 未配置")
		} else {
			utils.Infof("  数据库: %s", dbCfg.Database)
		}

		if err := backup.ValidateDriverConfig(&backup.DBConfig{
			Type:     dbCfg.Type,
			Host:     dbCfg.Host,
			Port:     dbCfg.Port,
			User:     dbCfg.User,
			Password: dbCfg.Password,
			Database: dbCfg.Database,
			SSLMode:  dbCfg.SSLMode,
			Extra:    dbCfg.Extra,
		}); err != nil {
			return fmt.Errorf("数据库 %s 的配置验证失败: %w", dbTypeKey, err)
		}
		utils.Infof("  ✓ 配置验证通过")
	}

	utils.Info("\n=== 配置文件验证通过 ===")
	return nil
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&configFilePath, "config", "c", "", "配置文件路径")
	rootCmd.PersistentFlags().StringVarP(&databaseType, "db-type", "t", "mysql", "数据库类型: mysql, postgresql, oracle, mssql")

	rootCmd.AddCommand(validateConfigCmd)
}
