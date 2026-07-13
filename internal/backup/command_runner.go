package backup

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/RealChuan/db-backup-restore/internal/logging"
	"github.com/RealChuan/db-backup-restore/pkg/shellexec"
)

// runOption 配置命令执行的选项
type runOption struct {
	env        []string               // 额外环境变量
	convertGBK bool                   // 是否 GBK→UTF8 转换
	stdin      io.Reader              // 标准输入
	onLine     shellexec.LineCallback // 流式输出回调
}

// runOptionFunc 函数式选项
type runOptionFunc func(*runOption)

// withEnv 设置额外环境变量
func withEnv(env []string) runOptionFunc {
	return func(o *runOption) { o.env = env }
}

// withGBKConversion 启用 GBK→UTF8 转换（Windows 上 MySQL/达梦工具需要）
func withGBKConversion() runOptionFunc {
	return func(o *runOption) { o.convertGBK = true }
}

// withStdin 设置标准输入
func withStdin(r io.Reader) runOptionFunc {
	return func(o *runOption) { o.stdin = r }
}

// withStreaming 设置流式输出回调（仅用于 runStreaming）
func withStreaming(onLine shellexec.LineCallback) runOptionFunc {
	return func(o *runOption) { o.onLine = onLine }
}

// buildCmdString 从 exec.Cmd 构建可读命令字符串
func buildCmdString(cmd *exec.Cmd) string {
	if len(cmd.Args) > 0 {
		return strings.Join(cmd.Args, " ")
	}
	return cmd.Path
}

// maskCmdString 对命令字符串中的密码进行脱敏
func maskCmdString(cmdStr string) string {
	return MaskPassword(cmdStr)
}

// runCapture 执行命令并捕获 stdout+stderr 到字符串。
// 成功时返回输出，失败时返回 CommandError。
func runCapture(ctx context.Context, toolName string, cmd *exec.Cmd, opts ...runOptionFunc) (string, error) {
	o := applyRunOpts(opts)
	applyOptsToCmd(cmd, &o)

	cmdStr := maskCmdString(buildCmdString(cmd))
	logging.DebugCtx(ctx, fmt.Sprintf("[命令执行] %s", cmdStr))

	if o.stdin != nil {
		cmd.Stdin = o.stdin
	}

	output, err := shellexec.ExecCommandWithEncoding(cmd, o.convertGBK)
	if err != nil {
		stderrOutput := extractStderr(err)
		logging.ErrorCtx(ctx, fmt.Sprintf("%s 执行失败", toolName), "cmd", cmdStr, "error", err, "stdout", truncateOutput(output, 500))
		return output, &CommandError{
			Tool:    toolName,
			Cmd:     cmdStr,
			Stdout:  truncateOutput(output, 2000),
			Stderr:  stderrOutput,
			Message: fmt.Sprintf("%s 执行失败", toolName),
			Cause:   err,
		}
	}

	if output != "" {
		logging.DebugCtx(ctx, fmt.Sprintf("%s 执行输出", toolName), "output", truncateOutput(output, 500))
	}
	return output, nil
}

// runToFile 执行命令，stdout 写入指定文件，捕获 stderr。
// 失败时返回 CommandError。
// 适用于 mysqldump/pg_dump/dexp 等导出类命令，避免大输出占用内存。
func runToFile(ctx context.Context, toolName string, cmd *exec.Cmd, outputPath string, opts ...runOptionFunc) error {
	o := applyRunOpts(opts)
	applyOptsToCmd(cmd, &o)

	cmdStr := maskCmdString(buildCmdString(cmd))
	logging.DebugCtx(ctx, fmt.Sprintf("[命令执行] %s", cmdStr))

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("创建输出文件失败: %w", err)
	}
	defer file.Close()

	cmd.Stdout = file

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return &CommandError{
			Tool:    toolName,
			Cmd:     cmdStr,
			Message: fmt.Sprintf("%s 执行失败", toolName),
			Cause:   err,
		}
	}

	stderrBytes, err := io.ReadAll(stderr)
	if err != nil {
		return fmt.Errorf("读取命令错误输出失败: %w", err)
	}

	stderrOutput, _ := shellexec.ConvertGBKToUTF8(stderrBytes)

	if err := cmd.Wait(); err != nil {
		logging.ErrorCtx(ctx, fmt.Sprintf("%s 执行失败", toolName), "cmd", cmdStr, "stderr", truncateOutput(stderrOutput, 500))
		return &CommandError{
			Tool:    toolName,
			Cmd:     cmdStr,
			Stderr:  stderrOutput,
			Message: fmt.Sprintf("%s 执行失败", toolName),
			Cause:   err,
		}
	}

	if stderrOutput != "" {
		logging.DebugCtx(ctx, fmt.Sprintf("%s stderr 输出", toolName), "output", truncateOutput(stderrOutput, 500))
	}
	return nil
}

// runStreaming 执行命令，流式输出通过 onLine 回调。
// 成功时返回 stdout 完整内容，失败时返回 CommandError。
// 适用于 dmrman/disql/rman 等长时间运行的交互式命令。
func runStreaming(ctx context.Context, toolName string, cmd *exec.Cmd, opts ...runOptionFunc) (string, error) {
	o := applyRunOpts(opts)
	applyOptsToCmd(cmd, &o)

	cmdStr := maskCmdString(buildCmdString(cmd))
	logging.DebugCtx(ctx, fmt.Sprintf("[命令执行] %s", cmdStr))

	if o.stdin != nil {
		cmd.Stdin = o.stdin
	}

	output, err := shellexec.ExecCommandStreaming(cmd, o.convertGBK, o.onLine)
	if err != nil {
		stderrOutput := extractStderr(err)
		logging.ErrorCtx(ctx, fmt.Sprintf("%s 执行失败", toolName), "cmd", cmdStr, "error", err, "stdout", truncateOutput(output, 500))
		return output, &CommandError{
			Tool:    toolName,
			Cmd:     cmdStr,
			Stdout:  truncateOutput(output, 2000),
			Stderr:  stderrOutput,
			Message: fmt.Sprintf("%s 执行失败", toolName),
			Cause:   err,
		}
	}
	return output, nil
}

// applyRunOpts 应用函数式选项
func applyRunOpts(opts []runOptionFunc) runOption {
	var o runOption
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// applyOptsToCmd 将选项应用到 exec.Cmd
func applyOptsToCmd(cmd *exec.Cmd, o *runOption) {
	if len(o.env) > 0 {
		cmd.Env = append(os.Environ(), o.env...)
	}
	if o.stdin != nil {
		cmd.Stdin = o.stdin
	}
}

// extractStderr 从 shellexec 返回的错误中提取 stderr 内容
// shellexec.ExecCommandStreaming 的错误格式: "命令执行失败: ..., stderr: ..."
func extractStderr(err error) string {
	if err == nil {
		return ""
	}
	errMsg := err.Error()
	if idx := strings.LastIndex(errMsg, "stderr: "); idx >= 0 {
		return strings.TrimSpace(errMsg[idx+8:])
	}
	return ""
}

// truncateOutput 截断输出用于日志显示，避免大输出刷屏
func truncateOutput(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + fmt.Sprintf("... (截断，总长度 %d)", len(s))
}
