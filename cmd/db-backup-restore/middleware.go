package main

import (
	"github.com/spf13/cobra"

	"github.com/RealChuan/db-backup-restore/internal/logging"
)

// WrapCommand 为命令添加错误处理包装
func WrapCommand(cmd *cobra.Command) {
	if cmd.RunE != nil {
		originalRunE := cmd.RunE
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			logging.Debug("开始执行命令", "cmd", cmd.Name())

			err := originalRunE(cmd, args)
			if err != nil {
				logging.Debug("命令执行失败", "cmd", cmd.Name(), "error", err)
				return err
			}

			logging.Debug("命令执行成功", "cmd", cmd.Name())
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
