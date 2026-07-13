package backup

import (
	"path/filepath"
	"runtime"
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
	// 使用跨平台绝对路径
	var absPath1, absPath2, absPath3 string
	if runtime.GOOS == "windows" {
		absPath1 = `C:\tmp\backup.sql`
		absPath2 = `C:\tmp\back'up.sql`
		absPath3 = `C:\tmp\backup.bak`
	} else {
		absPath1 = `/tmp/backup.sql`
		absPath2 = `/tmp/back'up.sql`
		absPath3 = `/tmp/backup.bak`
	}

	tests := []struct {
		name       string
		path       string
		allowedExt []string
		wantErr    bool
	}{
		{"绝对路径SQL", absPath1, nil, false},
		{"空路径", "", nil, true},
		{"含单引号", absPath2, nil, true},
		{"相对路径", "backup.sql", nil, true},
		{"扩展名检查通过", absPath3, []string{".bak", ".trn"}, false},
		{"扩展名检查失败", filepath.Join(filepath.VolumeName(absPath1), filepath.FromSlash("/tmp/backup.txt")), []string{".bak", ".trn"}, true},
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
	backupDir := filepath.FromSlash("/backup")
	tests := []struct {
		name      string
		path      string
		backupDir string
		wantErr   bool
	}{
		{"路径在备份目录下", filepath.FromSlash("/backup/db1/backup.sql"), backupDir, false},
		{"路径等于备份目录", backupDir, backupDir, false},
		{"路径不在备份目录下", filepath.FromSlash("/other/backup.sql"), backupDir, true},
		{"备份目录为空", filepath.FromSlash("/backup/backup.sql"), "", true},
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

func TestEscapeDamengRMANString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"无特殊字符", `/opt/backup`, `/opt/backup`},
		{"包含单引号", `/opt/'backup'`, `/opt/''backup''`},
		{"多个单引号", `'a'b'c'`, `''a''b''c''`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeDamengRMANString(tt.input); got != tt.want {
				t.Errorf("escapeDamengRMANString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeSCN(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"valid scn", "123456789", 123456789, false},
		{"empty", "", 0, true},
		{"negative", "-1", 0, true},
		{"zero", "0", 0, true},
		{"non_numeric", "abc", 0, true},
		{"with_spaces", " 123 ", 123, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := sanitizeSCN(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("sanitizeSCN(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("sanitizeSCN(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeSeq(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"valid seq", "100", 100, false},
		{"empty", "", 0, true},
		{"negative", "-5", 0, true},
		{"zero", "0", 0, true},
		{"non_numeric", "1a2b", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := sanitizeSeq(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("sanitizeSeq(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("sanitizeSeq(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeLSN(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"valid lsn", "99999", 99999, false},
		{"empty", "", 0, true},
		{"negative", "-1", 0, true},
		{"zero", "0", 0, true},
		{"non_numeric", "abc", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := sanitizeLSN(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("sanitizeLSN(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("sanitizeLSN(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeDamengBackupPath(t *testing.T) {
	t.Run("有效路径", func(t *testing.T) {
		validPath := "/opt/dmdbms/backup"
		if runtime.GOOS == "windows" {
			validPath = `C:\opt\dmdbms\backup`
		}
		err := sanitizeDamengBackupPath(validPath)
		if err != nil {
			t.Errorf("有效路径不应报错，得到: %v", err)
		}
	})

	t.Run("空路径", func(t *testing.T) {
		err := sanitizeDamengBackupPath("")
		if err == nil {
			t.Error("空路径应报错")
		}
	})

	t.Run("相对路径", func(t *testing.T) {
		err := sanitizeDamengBackupPath("relative/path")
		if err == nil {
			t.Error("相对路径应报错")
		}
	})

	t.Run("包含分号", func(t *testing.T) {
		err := sanitizeDamengBackupPath("/opt/backup;rm -rf /")
		if err == nil {
			t.Error("包含分号的路径应报错")
		}
	})

	t.Run("包含反引号", func(t *testing.T) {
		err := sanitizeDamengBackupPath("/opt/`whoami`")
		if err == nil {
			t.Error("包含反引号的路径应报错")
		}
	})

	t.Run("包含美元符号", func(t *testing.T) {
		err := sanitizeDamengBackupPath("/opt/$HOME/backup")
		if err == nil {
			t.Error("包含美元符号的路径应报错")
		}
	})

	t.Run("包含单引号", func(t *testing.T) {
		err := sanitizeDamengBackupPath("/opt/backup'")
		if err == nil {
			t.Error("包含单引号的路径应报错")
		}
	})
}

func TestValidateDamengPassword(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"正常密码", "MyPassword123", false},
		{"含井号", "Pass#word!", false},
		{"含美元符", "My$ecret", false},
		{"含感叹号", "P@ssword", true}, // @ 是危险字符
		{"含斜杠", "Pass/word", true},
		{"空密码", "", false},
		{"仅含特殊字符但无危险字符", "#$%^&*!", false},
		{"含at符号和斜杠", "a/b@c", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateDamengPassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDamengPassword(%q) error = %v, wantErr %v", tt.password, err, tt.wantErr)
			}
		})
	}
}

func TestValidateRemapSchema(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		remap   string
		wantErr bool
	}{
		{"有效映射", "HR:HR_NEW", false},
		{"有效映射含下划线", "SCHEMA_A:SCHEMA_B", false},
		{"缺少冒号", "HRHR", true},
		{"空源模式", ":TARGET", true},
		{"空目标模式", "SOURCE:", true},
		{"两端为空", ":", true},
		{"源模式含危险字符", "HR';DROP:TARGET", true},
		{"目标模式含危险字符", "HR:TAR;GET", true},
		{"空字符串", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateRemapSchema(tt.remap)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRemapSchema(%q) error = %v, wantErr %v", tt.remap, err, tt.wantErr)
			}
		})
	}
}
