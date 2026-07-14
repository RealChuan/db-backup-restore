package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

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
		writeDataText(w, result.Data, "  ", result.Operation)
	} else {
		fmt.Fprintf(w, "%s 失败: %s\n", result.Operation, result.Error)
	}
	return nil
}

// writeDataText 渲染 Data 字段。
// info 操作使用多行逐字段格式，其他操作使用单行 key=value 格式。
func writeDataText(w io.Writer, data map[string]interface{}, indent string, operation string) {
	if len(data) == 0 {
		return
	}
	if operation == OpInfo {
		// info 保持多行逐字段展示
		for _, key := range sortedKeys(data) {
			val := data[key]
			writeDataEntry(w, key, val, indent)
		}
		return
	}
	writeDataOneline(w, data, indent)
}

// writeDataOneline 将 Data 字段渲染为紧凑格式。
// 简单 key=value 对在同一行用逗号分隔；
// 列表中的 map 项各占一行（key=key1=val1, key2=val2, ...），
// 列表中的非 map 值在同一行用逗号拼接。
func writeDataOneline(w io.Writer, data map[string]interface{}, indent string) {
	if len(data) == 0 {
		return
	}
	keys := sortedKeys(data)
	// 先收集简单值和列表值
	var simpleParts []string
	for _, k := range keys {
		v := data[k]
		switch tv := v.(type) {
		case []interface{}:
			if len(tv) > 0 {
				if _, ok := tv[0].(map[string]interface{}); ok {
					// 列表中的 map 项：逐行输出（不带列表 key 前缀）
					for _, item := range tv {
						m := item.(map[string]interface{})
						fmt.Fprintf(w, "%s%s\n", indent, formatFlatMap(m))
					}
				} else {
					simpleParts = append(simpleParts, fmt.Sprintf("%s=%s", k, formatValue(tv)))
				}
			} else {
				simpleParts = append(simpleParts, fmt.Sprintf("%s=", k))
			}
		default:
			simpleParts = append(simpleParts, fmt.Sprintf("%s=%s", k, formatValue(v)))
		}
	}
	if len(simpleParts) > 0 {
		fmt.Fprintf(w, "%s%s\n", indent, strings.Join(simpleParts, ", "))
	}
}

// formatValue 将值格式化为单行友好的字符串。
// 列表值用逗号拼接（map 项递归展平），嵌套 map 用 key.subkey=value 展平，其他值直接输出。
func formatValue(val interface{}) string {
	switch v := val.(type) {
	case []interface{}:
		items := make([]string, 0, len(v))
		for _, item := range v {
			items = append(items, formatValue(item))
		}
		return strings.Join(items, ",")
	case map[string]interface{}:
		return formatFlatMap(v)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// formatFlatMap 将嵌套 map 展平为逗号分隔的 key=value 格式。
func formatFlatMap(m map[string]interface{}) string {
	keys := sortedKeys(m)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := m[k]
		switch iv := v.(type) {
		case []interface{}:
			items := make([]string, 0, len(iv))
			for _, item := range iv {
				items = append(items, fmt.Sprintf("%v", item))
			}
			parts = append(parts, fmt.Sprintf("%s=%s", k, strings.Join(items, ",")))
		case map[string]interface{}:
			parts = append(parts, formatFlatMapWithPrefix(k, iv))
		default:
			parts = append(parts, fmt.Sprintf("%s=%v", k, iv))
		}
	}
	return strings.Join(parts, ", ")
}

// formatFlatMapWithPrefix 将嵌套 map 展平为 prefix.subkey=value 格式。
func formatFlatMapWithPrefix(prefix string, m map[string]interface{}) string {
	keys := sortedKeys(m)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		fullKey := prefix + "." + k
		v := m[k]
		switch iv := v.(type) {
		case []interface{}:
			items := make([]string, 0, len(iv))
			for _, item := range iv {
				items = append(items, fmt.Sprintf("%v", item))
			}
			parts = append(parts, fmt.Sprintf("%s=%s", fullKey, strings.Join(items, ",")))
		case map[string]interface{}:
			parts = append(parts, formatFlatMapWithPrefix(fullKey, iv))
		default:
			parts = append(parts, fmt.Sprintf("%s=%v", fullKey, iv))
		}
	}
	return strings.Join(parts, ", ")
}

// writeDataEntry 渲染单个 Data 条目（用于 info 操作的多行格式）。
func writeDataEntry(w io.Writer, key string, val interface{}, indent string) {
	switch v := val.(type) {
	case []interface{}:
		if len(v) == 0 {
			fmt.Fprintf(w, "%s%s: (空)\n", indent, key)
			return
		}
		fmt.Fprintf(w, "%s%s:\n", indent, key)
		for _, item := range v {
			fmt.Fprintf(w, "%s  - %v\n", indent, item)
		}
	case map[string]interface{}:
		// 嵌套 map 用 key.subkey 展平
		writeFlatMap(w, key, v, indent)
	default:
		fmt.Fprintf(w, "%s%s: %v\n", indent, key, val)
	}
}

// writeFlatMap 将嵌套 map 展平为 key.subkey: value 格式（用于 info 操作的多行格式）。
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

// sortedKeys 返回 map 的排序后 key 列表。
func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
