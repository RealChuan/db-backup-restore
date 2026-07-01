package app

import (
	"bytes"
	"testing"

	"github.com/RealChuan/db-backup-restore/internal/backup"
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
	// Data keys are sorted alphabetically
	want := "backup: 备份成功\n  duration: 12.3s\n  file: /backup/mysql_20260701.sql\n  size: 1.5 MB\n"
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
	want := "list_databases: 共 3 个数据库\n  databases:\n    - db_alpha\n    - db_beta\n    - db_gamma\n"
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
	want := "list_databases: 共 0 个数据库\n  databases: (空)\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOutputWriter_Text_DataNestedMap(t *testing.T) {
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
	// Data keys sorted: detail.size, detail.type, id
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
	want := "list: 共 1 个备份\n  backups:\n      id: bs_001\n      type: logical\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
