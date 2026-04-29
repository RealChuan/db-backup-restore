package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"db-backup-restore/internal/backup"
	"db-backup-restore/pkg/utils"
)

// DatabaseBackupFunc 数据库备份操作的函数类型
type DatabaseBackupFunc func(ctx context.Context, db backup.DatabaseBackup) error

// withDatabaseBackup 封装数据库备份操作的公共逻辑
func withDatabaseBackup(ctx context.Context, operation string, fn DatabaseBackupFunc) error {
	dbCfg, err := appConfig.GetDBConfig(databaseType)
	if err != nil {
		utils.AuditLog(operation, databaseType, "failed", "获取数据库配置失败: "+err.Error())
		return fmt.Errorf("获取数据库配置失败: %w", err)
	}

	db, err := backup.NewBackup(dbCfg)
	if err != nil {
		utils.AuditLog(operation, databaseType, "failed", "创建数据库备份实例失败: "+err.Error())
		return fmt.Errorf("创建数据库备份实例失败: %w", err)
	}
	defer db.Close()

	return fn(ctx, db)
}

// RunEWithErrorHandling 包装命令执行函数，统一处理错误
func RunEWithErrorHandling(command string, fn func(ctx context.Context) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		utils.Infof("开始执行命令: %s", command)

		err := fn(cmd.Context())
		if err != nil {
			utils.Errorf("命令 [%s] 执行失败: %v", command, err)
			return err
		}

		utils.Infof("命令 [%s] 执行成功", command)
		return nil
	}
}

// WrapCommand 为命令添加错误处理包装
func WrapCommand(cmd *cobra.Command) {
	if cmd.RunE != nil {
		originalRunE := cmd.RunE
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			utils.Infof("开始执行命令: %s", cmd.Name())

			err := originalRunE(cmd, args)
			if err != nil {
				utils.Errorf("命令 [%s] 执行失败: %v", cmd.Name(), err)
				return err
			}

			utils.Infof("命令 [%s] 执行成功", cmd.Name())
			return nil
		}
	}

	for _, subCmd := range cmd.Commands() {
		WrapCommand(subCmd)
	}
}

// SetupCommandErrorHandling 为所有命令设置错误处理
func SetupCommandErrorHandling(root *cobra.Command) {
	WrapCommand(root)
}
