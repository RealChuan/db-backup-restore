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

// ExecCommand 执行命令并处理输出的字符编码
func ExecCommand(ctx context.Context, cmd *exec.Cmd) (string, error) {
	output, err := cmd.CombinedOutput()
	// 尝试将GBK编码转换为UTF-8，解决乱码问题
	convertedOutput, convertErr := ConvertGBKToUTF8(output)
	if convertErr != nil {
		// 如果转换失败，使用原始输出
		convertedOutput = string(output)
	}
	if err != nil {
		return convertedOutput, fmt.Errorf("命令执行失败: %w, 输出: %s", err, convertedOutput)
	}
	return convertedOutput, nil
}
