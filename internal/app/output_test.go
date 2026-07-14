package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/RealChuan/db-backup-restore/internal/backup"
	"github.com/stretchr/testify/assert"
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
		Message:   "备份成功",
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
