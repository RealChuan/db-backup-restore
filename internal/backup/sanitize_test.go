package backup

import (
	"testing"
)

func TestSanitizeDatabaseName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"正常名称", "mydb", false},
		{"空名称", "", true},
		{"含单引号", "my'db", true},
		{"含双引号", `my"db`, true},
		{"含分号", "my;db", true},
		{"含反斜杠", "my\\db", true},
		{"含方括号", "my[db", true},
		{"中文数据库名", "我的数据库", false},
		{"双横线不是危险字符", "my--db", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sanitizeDatabaseName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("sanitizeDatabaseName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestSanitizeBackupPath(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		allowedExt []string
		wantErr    bool
	}{
		{"绝对路径SQL", `C:\tmp\backup.sql`, nil, false},
		{"空路径", "", nil, true},
		{"含单引号", `C:\tmp\back'up.sql`, nil, true},
		{"相对路径", "backup.sql", nil, true},
		{"扩展名检查通过", `C:\tmp\backup.bak`, []string{".bak", ".trn"}, false},
		{"扩展名检查失败", `C:\tmp\backup.txt`, []string{".bak", ".trn"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sanitizeBackupPath(tt.path, tt.allowedExt...)
			if (err != nil) != tt.wantErr {
				t.Errorf("sanitizeBackupPath(%q, %v) error = %v, wantErr %v", tt.path, tt.allowedExt, err, tt.wantErr)
			}
		})
	}
}

func TestSanitizePositiveInt(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"正整数", "42", 42, false},
		{"空字符串", "", 0, true},
		{"零", "0", 0, true},
		{"负数", "-1", 0, true},
		{"非数字", "abc", 0, true},
		{"带空格", " 10 ", 10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sanitizePositiveInt(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("sanitizePositiveInt(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("sanitizePositiveInt(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeDateLiteral(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"有效日期", "2024-01-15 10:30:00", false},
		{"空字符串", "", true},
		{"含单引号", "2024-01-15' 10:30:00", true},
		{"格式错误", "2024/01/15 10:30:00", true},
		{"带空格", " 2024-01-15 10:30:00 ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sanitizeDateLiteral(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("sanitizeDateLiteral(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestEscapeOracleRMANString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"无单引号", "hello", "hello"},
		{"一个单引号", "it's", "it''s"},
		{"多个单引号", "a'b'c", "a''b''c"},
		{"空字符串", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeOracleRMANString(tt.input)
			if got != tt.want {
				t.Errorf("escapeOracleRMANString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeOracleBackupID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"有效ID", "backup_123", false},
		{"空ID", "", true},
		{"含特殊字符", "backup$123", true},
		{"纯字母数字", "ABC123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sanitizeOracleBackupID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("sanitizeOracleBackupID(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestMustBeUnderBackupDir(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		backupDir string
		wantErr   bool
	}{
		{"路径在备份目录下", `/backup/db1/backup.sql`, `/backup`, false},
		{"路径等于备份目录", `/backup`, `/backup`, false},
		{"路径不在备份目录下", `/other/backup.sql`, `/backup`, true},
		{"备份目录为空", `/backup/backup.sql`, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mustBeUnderBackupDir(tt.path, tt.backupDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("mustBeUnderBackupDir(%q, %q) error = %v, wantErr %v", tt.path, tt.backupDir, err, tt.wantErr)
			}
		})
	}
}
