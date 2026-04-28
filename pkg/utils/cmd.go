package utils

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// ConvertGBKToUTF8 将GBK编码的字节数组转换为UTF-8编码的字符串
// 仅在Windows系统上执行转换，其他系统直接返回原始字符串
func ConvertGBKToUTF8(data []byte) (string, error) {
	// 仅在Windows系统上进行转换
	if runtime.GOOS != "windows" {
		return string(data), nil
	}

	// 使用GBK解码器将数据转换为UTF-8
	reader := transform.NewReader(strings.NewReader(string(data)), simplifiedchinese.GBK.NewDecoder())
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return string(data), err
	}
	return string(decoded), nil
}

// ExecCommand 执行命令并处理输出的字符编码（只返回 stdout）
// 无论命令是否成功，都返回 stdout 和 error
// 默认进行 GBK 转 UTF8 转换
func ExecCommand(ctx context.Context, cmd *exec.Cmd) (string, error) {
	return ExecCommandWithEncoding(ctx, cmd, true)
}

// ExecCommandWithEncoding 执行命令并处理输出的字符编码
// convertGBK: 是否将 GBK 编码转换为 UTF8
func ExecCommandWithEncoding(ctx context.Context, cmd *exec.Cmd, convertGBK bool) (string, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}

	if err := cmd.Start(); err != nil {
		return "", err
	}

	stdoutBytes, _ := io.ReadAll(stdout)
	stderrBytes, _ := io.ReadAll(stderr)

	var output, stderrOutput string
	if convertGBK {
		output, _ = ConvertGBKToUTF8(stdoutBytes)
		stderrOutput, _ = ConvertGBKToUTF8(stderrBytes)
	} else {
		output = string(stdoutBytes)
		stderrOutput = string(stderrBytes)
	}

	if stderrOutput != "" {
		Warnf("命令执行警告: %s", stderrOutput)
	}

	if err := cmd.Wait(); err != nil {
		return output, fmt.Errorf("命令执行失败: %w, stderr: %s", err, stderrOutput)
	}

	return output, nil
}
