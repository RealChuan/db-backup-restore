package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/RealChuan/db-backup-restore/internal/backup"
)

// OperationResult 操作结果（用于结构化输出）
type OperationResult struct {
	Success   bool                   `json:"success"`
	Operation string                 `json:"operation"`
	DBType    string                 `json:"db_type,omitempty"`
	Message   string                 `json:"message,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// OutputWriter 输出写入器
type OutputWriter struct {
	format backup.OutputFormat
	writer io.Writer
}

// NewOutputWriter 创建输出写入器
func NewOutputWriter(format backup.OutputFormat) *OutputWriter {
	return &OutputWriter{
		format: format,
		writer: os.Stdout,
	}
}

// Write 写入操作结果
func (w *OutputWriter) Write(result *OperationResult) error {
	if w.format == backup.OutputFormatJSON {
		encoder := json.NewEncoder(w.writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}
	// text 格式
	return writeTextResult(w.writer, result)
}

// writeTextResult 将 OperationResult 格式化为 text 并写入。
func writeTextResult(w io.Writer, result *OperationResult) error {
	if result.Success {
		msg := result.Message
		if msg == "" {
			msg = "完成"
		}
		fmt.Fprintf(w, "%s: %s\n", result.Operation, msg)
		writeDataText(w, result.Data, "  ")
	} else {
		fmt.Fprintf(w, "%s 失败: %s\n", result.Operation, result.Error)
	}
	return nil
}

// writeDataText 递归渲染 Data 字段为缩进的 key-value 格式。
func writeDataText(w io.Writer, data map[string]interface{}, indent string) {
	if len(data) == 0 {
		return
	}
	for _, key := range sortedKeys(data) {
		val := data[key]
		writeDataEntry(w, key, val, indent)
	}
}

// writeDataEntry 渲染单个 Data 条目。
func writeDataEntry(w io.Writer, key string, val interface{}, indent string) {
	switch v := val.(type) {
	case []interface{}:
		if len(v) == 0 {
			fmt.Fprintf(w, "%s%s: (空)\n", indent, key)
			return
		}
		fmt.Fprintf(w, "%s%s:\n", indent, key)
		for i, item := range v {
			switch iv := item.(type) {
			case map[string]interface{}:
				// 列表中的 map 项：缩进展平，条目之间加空行分隔
				if i > 0 {
					fmt.Fprintln(w)
				}
				writeFlatMapEntries(w, iv, indent+"    ")
			default:
				fmt.Fprintf(w, "%s  - %v\n", indent, item)
			}
		}
	case map[string]interface{}:
		// 嵌套 map 用 key.subkey 展平
		writeFlatMap(w, key, v, indent)
	default:
		fmt.Fprintf(w, "%s%s: %v\n", indent, key, val)
	}
}

// writeFlatMap 将嵌套 map 展平为 key.subkey: value 格式（仅一级）。
func writeFlatMap(w io.Writer, prefix string, m map[string]interface{}, indent string) {
	keys := sortedKeys(m)
	for _, k := range keys {
		fullKey := prefix + "." + k
		val := m[k]
		switch v := val.(type) {
		case []interface{}:
			writeDataEntry(w, fullKey, v, indent)
		case map[string]interface{}:
			writeFlatMap(w, fullKey, v, indent)
		default:
			fmt.Fprintf(w, "%s%s: %v\n", indent, fullKey, val)
		}
	}
}

// writeFlatMapEntries 渲染 map 的各个条目（用于列表中的 map 项）。
// 与 writeFlatMap 不同，这里不带父 key 前缀，直接输出 key: value。
func writeFlatMapEntries(w io.Writer, m map[string]interface{}, indent string) {
	keys := sortedKeys(m)
	for _, k := range keys {
		val := m[k]
		switch v := val.(type) {
		case []interface{}:
			writeDataEntry(w, k, v, indent)
		case map[string]interface{}:
			writeFlatMap(w, k, v, indent)
		default:
			fmt.Fprintf(w, "%s%s: %v\n", indent, k, val)
		}
	}
}

// sortedKeys 返回 map 的排序后 key 列表。
func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
