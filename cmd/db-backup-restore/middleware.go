package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/RealChuan/db-backup-restore/internal/logging"
)

// RunEWithErrorHandling 包装命令执行函数，统一处理错误
func RunEWithErrorHandling(command string, fn func() error) func(_ *cobra.Command, _ []string) error {
	return func(_ *cobra.Command, _ []string) error {
		logging.Info(fmt.Sprintf("开始执行命令: %s", command))

		err := fn()
		if err != nil {
			logging.Error(fmt.Sprintf("命令 [%s] 执行失败: %v", command, err))
			return err
		}

		logging.Info(fmt.Sprintf("命令 [%s] 执行成功", command))
		return nil
	}
}

// WrapCommand 为命令添加错误处理包装
func WrapCommand(cmd *cobra.Command) {
	if cmd.RunE != nil {
		originalRunE := cmd.RunE
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			logging.Info(fmt.Sprintf("开始执行命令: %s", cmd.Name()))

			err := originalRunE(cmd, args)
			if err != nil {
				logging.Error(fmt.Sprintf("命令 [%s] 执行失败: %v", cmd.Name(), err))
				return err
			}

			logging.Info(fmt.Sprintf("命令 [%s] 执行成功", cmd.Name()))
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
