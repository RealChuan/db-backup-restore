package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBaseBackup_ListDatabases_NotSupported(t *testing.T) {
	b := NewBaseBackup(&DBConfig{Type: DBTypeOracle})
	_, err := b.ListDatabases(context.Background())
	if err == nil {
		t.Fatal("期望返回错误，但返回了 nil")
	}

	var be *BackupError
	if !errors.As(err, &be) {
		t.Fatalf("期望 *BackupError，实际 %T", err)
	}

	if be.Type != ErrorTypeNotSupported {
		t.Errorf("Type = %v, want %v", be.Type, ErrorTypeNotSupported)
	}
	if be.Op != "ListDatabases" {
		t.Errorf("Op = %v, want ListDatabases", be.Op)
	}
	if be.DBType != DBTypeOracle {
		t.Errorf("DBType = %v, want %v", be.DBType, DBTypeOracle)
	}
}

// TestBaseBackup_DefaultNotSupported 验证所有默认 NotSupported 实现返回正确错误。
func TestBaseBackup_DefaultNotSupported(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	b := NewBaseBackup(&DBConfig{Type: DBTypeOracle})

	tests := []struct {
		name string
		op   string
		err  error
	}{
		{"RegisterBackup", "RegisterBackup", b.RegisterBackup(ctx, "/tmp/x")},
		{"UnregisterBackup", "UnregisterBackup", b.UnregisterBackup(ctx, "/tmp/x")},
		{"VerifyBackupStatus", "VerifyBackupStatus", b.VerifyBackupStatus(ctx)},
		{"DeleteInvalidBackups", "DeleteInvalidBackups", b.DeleteInvalidBackups(ctx)},
		{"ValidateBackup", "ValidateBackup", b.ValidateBackup(ctx, "/tmp/x")},
		{"EnableArchiveLogMode", "EnableArchiveLogMode", b.EnableArchiveLogMode(ctx, "/tmp/arch")},
		{"DisableArchiveLogMode", "DisableArchiveLogMode", b.DisableArchiveLogMode(ctx)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var be *BackupError
			if !errors.As(tt.err, &be) {
				t.Fatalf("%s 期望 *BackupError，实际 %T", tt.name, tt.err)
			}
			if be.Type != ErrorTypeNotSupported {
				t.Errorf("Type = %v, want %v", be.Type, ErrorTypeNotSupported)
			}
			if be.Op != tt.op {
				t.Errorf("Op = %v, want %v", be.Op, tt.op)
			}
			if be.DBType != DBTypeOracle {
				t.Errorf("DBType = %v, want %v", be.DBType, DBTypeOracle)
			}
		})
	}
}

// TestBaseBackup_Close 验证 Close 默认返回 nil。
func TestBaseBackup_Close(t *testing.T) {
	t.Parallel()
	b := NewBaseBackup(&DBConfig{Type: DBTypeMySQL})
	if err := b.Close(); err != nil {
		t.Errorf("Close() 期望 nil, got %v", err)
	}
}

// TestBaseBackup_GetConfig 验证 GetConfig 返回传入的配置。
func TestBaseBackup_GetConfig(t *testing.T) {
	t.Parallel()
	cfg := &DBConfig{Type: DBTypeMySQL, Host: "localhost", Port: 3306}
	b := NewBaseBackup(cfg)
	got := b.GetConfig()
	if got != cfg {
		t.Errorf("GetConfig() 返回的指针不一致：got %p, want %p", got, cfg)
	}
	if got.Type != DBTypeMySQL || got.Host != "localhost" || got.Port != 3306 {
		t.Errorf("GetConfig() 内容不匹配：got %+v", got)
	}
}

func TestBaseBackup_ParseDatabaseNames(t *testing.T) {
	t.Parallel()
	b := NewBaseBackup(&DBConfig{Type: DBTypeMySQL})

	tests := []struct {
		name         string
		databaseName string
		want         []string
	}{
		{"空字符串返回 nil", "", nil},
		{"all 返回 nil", "all", nil},
		{"单个数据库名", "mydb", []string{"mydb"}},
		{"多个数据库名", "db1,db2,db3", []string{"db1", "db2", "db3"}},
		{"带空格的多个数据库名", "db1, db2 , db3", []string{"db1", "db2", "db3"}},
		{"全空格被过滤", " , , ", nil},
		{"混合空和有效名称", "db1,,db2", []string{"db1", "db2"}},
		{"All 大写不视为关键字", "All", []string{"All"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := b.parseDatabaseNames(tt.databaseName)
			if len(got) != len(tt.want) {
				t.Errorf("parseDatabaseNames(%q) = %v, want %v", tt.databaseName, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseDatabaseNames(%q)[%d] = %q, want %q", tt.databaseName, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestBaseBackup_GetBackupDir(t *testing.T) {
	t.Parallel()
	b := NewBaseBackup(&DBConfig{Type: DBTypeMySQL})

	tests := []struct {
		name    string
		options []BackupOptions
		want    string
	}{
		{"nil 选项", nil, ""},
		{"空切片", []BackupOptions{}, ""},
		{"TargetPath 为空", []BackupOptions{{TargetPath: ""}}, ""},
		{"TargetPath 非空", []BackupOptions{{TargetPath: "/backup/mysql"}}, "/backup/mysql"},
		{"仅取第一个元素", []BackupOptions{
			{TargetPath: "/first"},
			{TargetPath: "/second"},
		}, "/first"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := b.getBackupDir(tt.options)
			if got != tt.want {
				t.Errorf("getBackupDir() = %q, want %q", got, tt.want)
			}
		})
	}
}

// restoreIdTest 定义 validateRestoreIdentifier 测试用例
type restoreIdTest struct {
	name       string
	identifier string // 空=自动从临时目录推导（除非 useEmptyID=true）
	backupType BackupType
	createFile bool // true=在临时目录创建文件, false=使用临时目录本身
	useEmptyID bool // true=显式传空字符串作为标识符
	wantErr    bool
	wantSub    string
	wantIsDir  bool
}

func TestBaseBackup_ValidateRestoreIdentifier(t *testing.T) {
	t.Parallel()
	b := NewBaseBackup(&DBConfig{Type: DBTypeMySQL})

	tests := []restoreIdTest{
		{name: "空标识符应报错", backupType: BackupTypeLogical, useEmptyID: true, wantErr: true, wantSub: "backup-identifier"},
		{name: "不存在路径应报错", identifier: "/nonexistent/path/backup.sql", backupType: BackupTypeLogical, wantErr: true, wantSub: "不可访问"},
		{name: "逻辑备份+文件通过", backupType: BackupTypeLogical, createFile: true, wantIsDir: false},
		{name: "空类型默认逻辑+文件通过", backupType: "", createFile: true, wantIsDir: false},
		{name: "逻辑备份+目录不匹配", backupType: BackupTypeLogical, wantErr: true, wantSub: "类型不匹配"},
		{name: "物理备份+目录通过", backupType: BackupTypePhysical, wantIsDir: true},
		{name: "物理备份+文件不匹配", backupType: BackupTypePhysical, createFile: true, wantErr: true, wantSub: "类型不匹配"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			identifier := tt.identifier
			if !tt.useEmptyID && identifier == "" {
				tmpDir := t.TempDir()
				if tt.createFile {
					identifier = filepath.Join(tmpDir, "backup.sql")
					if err := os.WriteFile(identifier, []byte("-- sql"), 0o644); err != nil {
						t.Fatalf("创建测试文件失败: %v", err)
					}
				} else {
					identifier = tmpDir
				}
			}
			isDir, err := b.validateRestoreIdentifier(identifier, tt.backupType)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRestoreIdentifier() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.wantSub != "" && !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("错误信息应包含 %q, got: %v", tt.wantSub, err)
			}
			if !tt.wantErr && isDir != tt.wantIsDir {
				t.Errorf("isDir = %v, want %v", isDir, tt.wantIsDir)
			}
		})
	}
}

func TestExtractDatabaseName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		backupFile string
		want       string
	}{
		{"标准格式", "/backup/mydb_20240115_103045.sql", "mydb"},
		{"标准格式仅文件名", "mydb_20240115_103045.sql", "mydb"},
		{"带下划线的数据库名", "/backup/my_db_1_20240115_103045.sql", "my_db_1"},
		{"非标准格式返回 base 名", "/backup/backup.sql", "backup.sql"},
		{"非标准格式无扩展名", "/backup/custom_backup", "custom_backup"},
		{"空路径返回当前目录标识", "", "."},
		{"Windows 路径", `C:\backup\mydb_20240115_103045.sql`, "mydb"},
		{"日期格式不完整不匹配", "mydb_20240115_10304.sql", "mydb_20240115_10304.sql"},
		{"扩展名非 sql 不匹配", "mydb_20240115_103045.bak", "mydb_20240115_103045.bak"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ExtractDatabaseName(tt.backupFile)
			if got != tt.want {
				t.Errorf("ExtractDatabaseName(%q) = %q, want %q", tt.backupFile, got, tt.want)
			}
		})
	}
}

func TestGenerateBackupFilename(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		dbName     string
		extension  string
		wantPrefix string
		wantSuffix string
	}{
		{"sql 扩展名", "mydb", "sql", "mydb_", ".sql"},
		{"bak 扩展名", "testdb", "bak", "testdb_", ".bak"},
		{"空扩展名", "mydb", "", "mydb_", "."},
		{"含下划线数据库名", "my_db", "sql", "my_db_", ".sql"},
		{"空数据库名", "", "sql", "_", ".sql"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := GenerateBackupFilename(tt.dbName, tt.extension)
			if !strings.HasPrefix(got, tt.wantPrefix) {
				t.Errorf("GenerateBackupFilename(%q, %q) = %q, 前缀应为 %q", tt.dbName, tt.extension, got, tt.wantPrefix)
			}
			if !strings.HasSuffix(got, tt.wantSuffix) {
				t.Errorf("GenerateBackupFilename(%q, %q) = %q, 后缀应为 %q", tt.dbName, tt.extension, got, tt.wantSuffix)
			}
			// 验证中间是 8 位日期 + 6 位时间（共 15 位，含下划线分隔）
			// 格式：name_YYYYMMDD_HHMMSS.ext
			middle := strings.TrimPrefix(got, tt.wantPrefix)
			middle = strings.TrimSuffix(middle, tt.wantSuffix)
			// middle 应为 "YYYYMMDD_HHMMSS"（15 字符）
			if len(middle) != 15 {
				t.Errorf("GenerateBackupFilename(%q, %q) 中间部分长度 = %d, want 15 (got: %q)", tt.dbName, tt.extension, len(middle), middle)
			}
		})
	}
}
