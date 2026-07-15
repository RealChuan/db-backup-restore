package app

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/RealChuan/db-backup-restore/internal/backup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputWriter_Text_SimpleSuccess(t *testing.T) {
	var buf bytes.Buffer
	w := &OutputWriter{format: backup.OutputFormatText, writer: &buf}
	result := &OperationResult{Success: true, Operation: "delete", Message: "删除成功"}
	if err := w.Write(result); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	want := "delete: 删除成功\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOutputWriter_Text_DataSimpleValues(t *testing.T) {
	var buf bytes.Buffer
	w := &OutputWriter{format: backup.OutputFormatText, writer: &buf}
	result := &OperationResult{
		Success:   true,
		Operation: "backup",
		Message:   MsgBackupSuccess,
		Data: map[string]interface{}{
			"file":     "/backup/mysql_20260701.sql",
			"size":     "1.5 MB",
			"duration": "12.3s",
		},
	}
	if err := w.Write(result); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	want := "backup: 备份成功\n  duration=12.3s, file=/backup/mysql_20260701.sql, size=1.5 MB\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOutputWriter_Text_DataListValues(t *testing.T) {
	var buf bytes.Buffer
	w := &OutputWriter{format: backup.OutputFormatText, writer: &buf}
	result := &OperationResult{
		Success:   true,
		Operation: "list_databases",
		DBType:    "mysql",
		Message:   "共 3 个数据库",
		Data:      map[string]interface{}{"databases": []interface{}{"db_alpha", "db_beta", "db_gamma"}},
	}
	if err := w.Write(result); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	want := "list_databases: 共 3 个数据库\n  databases=db_alpha,db_beta,db_gamma\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOutputWriter_Text_DataEmptyList(t *testing.T) {
	var buf bytes.Buffer
	w := &OutputWriter{format: backup.OutputFormatText, writer: &buf}
	result := &OperationResult{
		Success:   true,
		Operation: "list_databases",
		DBType:    "mssql",
		Message:   "共 0 个数据库",
		Data:      map[string]interface{}{"databases": []interface{}{}},
	}
	if err := w.Write(result); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	want := "list_databases: 共 0 个数据库\n  databases=\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOutputWriter_Text_DataNestedMap_InfoStaysMultiline(t *testing.T) {
	var buf bytes.Buffer
	w := &OutputWriter{format: backup.OutputFormatText, writer: &buf}
	result := &OperationResult{
		Success:   true,
		Operation: "info",
		Data: map[string]interface{}{
			"id":     "backup_001",
			"detail": map[string]interface{}{"type": "logical", "size": "10 MB"},
		},
	}
	if err := w.Write(result); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	// info 操作保持多行逐字段格式
	want := "info: 完成\n  detail.size: 10 MB\n  detail.type: logical\n  id: backup_001\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOutputWriter_Text_NoMessageNoData(t *testing.T) {
	var buf bytes.Buffer
	w := &OutputWriter{format: backup.OutputFormatText, writer: &buf}
	result := &OperationResult{Success: true, Operation: "verify_status"}
	if err := w.Write(result); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	want := "verify_status: 完成\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOutputWriter_Text_Failure(t *testing.T) {
	var buf bytes.Buffer
	w := &OutputWriter{format: backup.OutputFormatText, writer: &buf}
	result := &OperationResult{Success: false, Operation: "backup", Error: "连接超时"}
	if err := w.Write(result); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	want := "backup 失败: 连接超时\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOutputWriter_Text_DataListOfMaps(t *testing.T) {
	var buf bytes.Buffer
	w := &OutputWriter{format: backup.OutputFormatText, writer: &buf}
	result := &OperationResult{
		Success:   true,
		Operation: "list",
		Message:   "共 1 个备份",
		Data: map[string]interface{}{
			"backups": []interface{}{
				map[string]interface{}{"id": "bs_001", "type": "logical"},
			},
		},
	}
	if err := w.Write(result); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	want := "list: 共 1 个备份\n  id=bs_001, type=logical\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOutputWriter_Text_DataListOfMaps_EachOnOneLine(t *testing.T) {
	var buf bytes.Buffer
	w := &OutputWriter{format: backup.OutputFormatText, writer: &buf}
	result := &OperationResult{
		Success:   true,
		Operation: "list",
		Message:   "共 3 个备份",
		Data: map[string]interface{}{
			"backups": []interface{}{
				map[string]interface{}{"id": 706, "mode": "full"},
				map[string]interface{}{"id": 707, "mode": "archive"},
				map[string]interface{}{"id": 708, "mode": "full"},
			},
		},
	}
	if err := w.Write(result); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	output := buf.String()
	lines := strings.Split(output, "\n")

	// 验证每个备份占一行，包含 id 和 mode
	var backupLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "id=") {
			backupLines = append(backupLines, trimmed)
		}
	}
	assert.Equal(t, 3, len(backupLines), "应有 3 行备份信息")
	assert.Contains(t, backupLines[0], "id=706")
	assert.Contains(t, backupLines[0], "mode=full")
	assert.Contains(t, backupLines[1], "id=707")
	assert.Contains(t, backupLines[1], "mode=archive")
	assert.Contains(t, backupLines[2], "id=708")
	assert.Contains(t, backupLines[2], "mode=full")
}

func TestOutputWriter_Text_ValidateConfig(t *testing.T) {
	var buf bytes.Buffer
	w := &OutputWriter{format: backup.OutputFormatText, writer: &buf}
	result := &OperationResult{
		Success:   true,
		Operation: "validate_config",
		Message:   "配置验证通过",
		Data: map[string]interface{}{
			"databases_count": 3,
			"base_backup_dir": "/data/backup",
		},
	}
	if err := w.Write(result); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	want := "validate_config: 配置验证通过\n  base_backup_dir=/data/backup, databases_count=3\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOutputWriter_Text_ListDrivers(t *testing.T) {
	var buf bytes.Buffer
	w := &OutputWriter{format: backup.OutputFormatText, writer: &buf}
	result := &OperationResult{
		Success:   true,
		Operation: "list_drivers",
		Message:   "共 1 个驱动",
		Data: map[string]interface{}{
			"drivers": []interface{}{
				map[string]interface{}{
					"name":                   "mysql",
					"version":                "1.0.0",
					"description":            "MySQL driver",
					"supported_actions":      []interface{}{"backup", "restore"},
					"supported_backup_types": []interface{}{"logical", "physical"},
				},
			},
		},
	}
	if err := w.Write(result); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	want := "list_drivers: 共 1 个驱动\n  description=MySQL driver, name=mysql, supported_actions=backup,restore, supported_backup_types=logical,physical, version=1.0.0\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// decodeJSON 辅助函数：将缓冲区内容解码为 OperationResult
func decodeJSON(t *testing.T, b []byte) OperationResult {
	t.Helper()
	var got OperationResult
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(b), &got), "JSON 解析失败: %s", b)
	return got
}

func TestOutputWriter_JSON_SuccessWithAllFields(t *testing.T) {
	var buf bytes.Buffer
	w := &OutputWriter{format: backup.OutputFormatJSON, writer: &buf}
	result := &OperationResult{
		Success:   true,
		Operation: "backup",
		DBType:    "mysql",
		Message:   MsgBackupSuccess,
		Data:      map[string]interface{}{"file": "/backup/db.sql"},
	}
	require.NoError(t, w.Write(result))

	got := decodeJSON(t, buf.Bytes())
	assert.Equal(t, true, got.Success)
	assert.Equal(t, "backup", got.Operation)
	assert.Equal(t, "mysql", got.DBType)
	assert.Equal(t, MsgBackupSuccess, got.Message)
	assert.Equal(t, "/backup/db.sql", got.Data["file"])
	assert.Empty(t, got.Error, "成功时 Error 应为空（omitempty）")
}

func TestOutputWriter_JSON_FailureWithError(t *testing.T) {
	var buf bytes.Buffer
	w := &OutputWriter{format: backup.OutputFormatJSON, writer: &buf}
	result := &OperationResult{
		Success:   false,
		Operation: "restore",
		DBType:    "postgresql",
		Error:     "连接超时",
	}
	require.NoError(t, w.Write(result))

	got := decodeJSON(t, buf.Bytes())
	assert.Equal(t, false, got.Success)
	assert.Equal(t, "restore", got.Operation)
	assert.Equal(t, "连接超时", got.Error)
	// 失败时 Message 应为空（omitempty）
	assert.Empty(t, got.Message)
	// Data 为 nil 时 omitempty 应省略
	assert.Nil(t, got.Data)
}

func TestOutputWriter_JSON_OmitemptyBehavior(t *testing.T) {
	var buf bytes.Buffer
	w := &OutputWriter{format: backup.OutputFormatJSON, writer: &buf}
	// 最小字段：仅 Success 和 Operation（其他字段为零值，应被 omitempty 省略）
	result := &OperationResult{Success: true, Operation: "verify_status"}
	require.NoError(t, w.Write(result))

	// 验证零值字段被省略（不在 JSON 输出中）
	raw := buf.String()
	assert.Contains(t, raw, `"success": true`)
	assert.Contains(t, raw, `"operation": "verify_status"`)
	assert.NotContains(t, raw, `"db_type"`, "零值 DBType 应被 omitempty 省略")
	assert.NotContains(t, raw, `"message"`, "零值 Message 应被 omitempty 省略")
	assert.NotContains(t, raw, `"data"`, "nil Data 应被 omitempty 省略")
	assert.NotContains(t, raw, `"error"`, "零值 Error 应被 omitempty 省略")
}

func TestOutputWriter_JSON_EmptyDataOmitted(t *testing.T) {
	var buf bytes.Buffer
	w := &OutputWriter{format: backup.OutputFormatJSON, writer: &buf}
	// 非 nil 但空的 Data map - 由于 omitempty，应与 nil Data 行为一致（字段被省略）
	result := &OperationResult{
		Success:   true,
		Operation: "list",
		Data:      map[string]interface{}{},
	}
	require.NoError(t, w.Write(result))

	raw := buf.String()
	assert.NotContains(t, raw, `"data"`, "空 map 由于 omitempty 应被省略")

	got := decodeJSON(t, buf.Bytes())
	assert.Nil(t, got.Data, "空 map 被省略后，反序列化的 Data 应为 nil")
}
