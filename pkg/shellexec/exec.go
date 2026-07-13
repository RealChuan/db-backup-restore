package shellexec

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// windowsOS 常量，用于 GOOS 判断，避免 goconst 重复字符串告警
const windowsOS = "windows"

// ConvertGBKToUTF8 将GBK编码的字节数组转换为UTF-8编码的字符串
// 仅在Windows系统上执行转换，其他系统直接返回原始字符串
func ConvertGBKToUTF8(data []byte) (string, error) {
	// 仅在Windows系统上进行转换
	if runtime.GOOS != windowsOS {
		return string(data), nil
	}

	// 使用GBK解码器将数据转换为UTF-8
	reader := transform.NewReader(bytes.NewReader(data), simplifiedchinese.GBK.NewDecoder())
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return string(data), err
	}
	return string(decoded), nil
}

// LineCallback 是逐行输出的回调函数类型，line 为已解码的行内容
type LineCallback func(line string)

// ExecCommand 执行命令并处理输出的字符编码（只返回 stdout）
// 无论命令是否成功，都返回 stdout 和 error
// 默认进行 GBK 转 UTF8 转换
func ExecCommand(cmd *exec.Cmd) (string, error) {
	return ExecCommandWithEncoding(cmd, true)
}

// ExecCommandWithEncoding 执行命令并处理输出的字符编码
// convertGBK: 是否将 GBK 编码转换为 UTF8
func ExecCommandWithEncoding(cmd *exec.Cmd, convertGBK bool) (string, error) {
	return ExecCommandStreaming(cmd, convertGBK, nil)
}

// ExecCommandStreaming 执行命令，逐行流式读取 stdout/stderr 并通过回调实时输出。
// 相比 ExecCommand，本函数不会将全部输出缓存到内存，适用于长时间运行的命令（如数据库备份/还原）：
//   - onLine 回调实时接收 stdout 的每一行（已转码），可传入 nil 表示不需要逐行回调
//   - stderr 内容通过 slog.Warn 实时输出
//   - 返回值 output 为 stdout 的完整内容（已转码）
//
// convertGBK: 是否将 GBK 编码转换为 UTF8（Windows 上达梦/Oracle 工具常输出 GBK）
func ExecCommandStreaming(cmd *exec.Cmd, convertGBK bool, onLine LineCallback) (string, error) {
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

	// 构建 stdout 的解码 Reader
	var stdoutReader io.Reader = stdout
	if convertGBK && runtime.GOOS == "windows" {
		stdoutReader = transform.NewReader(stdout, simplifiedchinese.GBK.NewDecoder())
	}

	// 构建 stderr 的解码 Reader
	var stderrReader io.Reader = stderr
	if convertGBK && runtime.GOOS == "windows" {
		stderrReader = transform.NewReader(stderr, simplifiedchinese.GBK.NewDecoder())
	}

	// 逐行读取 stdout，同时累积完整输出
	var outputBuilder strings.Builder
	stdoutDone := make(chan struct{})
	go func() {
		defer close(stdoutDone)
		scanner := bufio.NewScanner(stdoutReader)
		// 备份输出可能包含长行，放宽单行上限到 1MB
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			outputBuilder.WriteString(line)
			outputBuilder.WriteString("\n")
			if onLine != nil {
				onLine(line)
			}
		}
		if scanErr := scanner.Err(); scanErr != nil {
			slog.Warn("stdout 扫描出错", "error", scanErr)
		}
	}()

	// 逐行读取 stderr，通过 slog 实时输出
	stderrDone := make(chan struct{})
	var stderrBuilder strings.Builder
	go func() {
		defer close(stderrDone)
		scanner := bufio.NewScanner(stderrReader)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			stderrBuilder.WriteString(line)
			stderrBuilder.WriteString("\n")
			slog.Warn("命令 stderr", "line", line)
		}
		if scanErr := scanner.Err(); scanErr != nil {
			slog.Warn("stderr 扫描出错", "error", scanErr)
		}
	}()

	<-stdoutDone
	<-stderrDone

	stderrOutput := stderrBuilder.String()
	if err := cmd.Wait(); err != nil {
		return outputBuilder.String(), fmt.Errorf("命令执行失败: %w, stderr: %s", err, stderrOutput)
	}

	return outputBuilder.String(), nil
}
