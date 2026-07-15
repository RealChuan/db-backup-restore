package backup

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestMaskScript(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "Oracle RMAN 单引号密码脱敏",
			input: `SET ENCRYPTION IDENTIFIED BY 'secret123'`,
			want:  `SET ENCRYPTION IDENTIFIED BY '***'`,
		},
		{
			name:  "达梦 dmrman 双引号密码脱敏",
			input: `IDENTIFIED BY "dameng_pwd"`,
			want:  `IDENTIFIED BY "***"`,
		},
		{
			name:  "无敏感信息不变",
			input: `BACKUP DATABASE FULL TO '/backup/db'`,
			want:  `BACKUP DATABASE FULL TO '/backup/db'`,
		},
		{
			name:  "空字符串不变",
			input: ``,
			want:  ``,
		},
		{
			name:  "Oracle 完整脚本脱敏",
			input: `RUN { SET ENCRYPTION IDENTIFIED BY 'mykey' ON; BACKUP DATABASE; }`,
			want:  `RUN { SET ENCRYPTION IDENTIFIED BY '***' ON; BACKUP DATABASE; }`,
		},
		{
			name:  "多空格分隔仍匹配",
			input: `SET ENCRYPTION IDENTIFIED   BY   'pass'`,
			want:  `SET ENCRYPTION IDENTIFIED   BY   '***'`,
		},
		{
			name:  "空密码（连续引号）",
			input: `IDENTIFIED BY ''`,
			want:  `IDENTIFIED BY '***'`,
		},
		{
			name:  "同一脚本中 Oracle 和达梦混合",
			input: `SET ENCRYPTION IDENTIFIED BY 'oracle_key' && IDENTIFIED BY "dameng_key"`,
			want:  `SET ENCRYPTION IDENTIFIED BY '***' && IDENTIFIED BY "***"`,
		},
		{
			name:  "无 IDENTIFIED BY 关键字不变",
			input: `SET ENCRYPTION ON; BACKUP DATABASE;`,
			want:  `SET ENCRYPTION ON; BACKUP DATABASE;`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := MaskScript(tt.input); got != tt.want {
				t.Errorf("MaskScript(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMaskPassword(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "长格式 --password=SECRET",
			input: `mysqldump -u root --password=secret -h localhost db`,
			want:  `mysqldump -u root --password=*** -h localhost db`,
		},
		{
			name:  "短格式 -pSECRET",
			input: `mysql -u root -psecret -h localhost`,
			want:  `mysql -u root -p*** -h localhost`,
		},
		{
			name:  "达梦 USERID=user/password@host:port",
			input: `dexp USERID=SYSDBA/pass123@localhost:5236`,
			want:  `dexp USERID=SYSDBA/***@localhost:5236`,
		},
		{
			name:  "无密码参数不变",
			input: `pg_dump -h localhost mydb`,
			want:  `pg_dump -h localhost mydb`,
		},
		{
			name:  "空字符串不变",
			input: ``,
			want:  ``,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := MaskPassword(tt.input); got != tt.want {
				t.Errorf("MaskPassword(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// dataDirPathTest 共享的路径校验测试用例结构
type dataDirPathTest struct {
	name      string
	cleanPath string
	wantErr   bool
	wantSub   string
}

// runDataDirPathTests 执行 validateDataDirPath 表驱动测试
func runDataDirPathTests(t *testing.T, tests []dataDirPathTest) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateDataDirPath(tt.cleanPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDataDirPath(%q) error = %v, wantErr %v", tt.cleanPath, err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.wantSub != "" && !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("validateDataDirPath(%q) 错误信息应包含 %q, got: %v", tt.cleanPath, tt.wantSub, err)
			}
		})
	}
}

func TestValidateDataDirPath_Relative(t *testing.T) {
	t.Parallel()
	runDataDirPathTests(t, []dataDirPathTest{
		{"相对路径", "relative/path", true, "absolute"},
		{"单点相对路径", ".", true, "absolute"},
		{"双点相对路径", "..", true, "absolute"},
	})
}

func TestValidateDataDirPath_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	t.Parallel()
	runDataDirPathTests(t, []dataDirPathTest{
		{"合法路径", `C:\data\mysql`, false, ""},
		{"深层路径", `D:\apps\mysql\instance1`, false, ""},
		{"根目录 C:\\", `C:\`, true, "root directory"},
		{"系统目录 Windows", `C:\Windows`, true, "system directory"},
		{"系统目录 Windows 子目录", `C:\Windows\System32\drivers`, true, "system directory"},
		{"系统目录 Program Files", `C:\Program Files`, true, "system directory"},
		{"系统目录 ProgramData", `C:\ProgramData`, true, "system directory"},
		{"系统目录 ProgramData 子目录", `C:\ProgramData\MySQL\data`, true, "system directory"},
		{"大小写不敏感 c:\\windows", `c:\windows`, true, "system directory"},
	})
}

func TestValidateDataDirPath_Linux(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Linux only")
	}
	t.Parallel()
	runDataDirPathTests(t, []dataDirPathTest{
		{"合法路径", "/var/lib/mysql", false, ""},
		{"深层路径", "/data/mysql/instance1", false, ""},
		{"根目录 /", "/", true, "root directory"},
		{"系统目录 /etc", "/etc", true, "system directory"},
		{"系统目录 /etc 子目录", "/etc/mysql", true, "system directory"},
		{"系统目录 /usr", "/usr", true, "system directory"},
		{"系统目录 /usr 子目录", "/usr/local/mysql", true, "system directory"},
		{"系统目录 /bin", "/bin", true, "system directory"},
		{"系统目录 /sbin", "/sbin", true, "system directory"},
		{"系统目录 /boot", "/boot", true, "system directory"},
		{"系统目录 /dev", "/dev", true, "system directory"},
		{"系统目录 /proc", "/proc", true, "system directory"},
		{"系统目录 /sys", "/sys", true, "system directory"},
		{"大小写不敏感 /ETC", "/ETC", true, "system directory"},
	})
}

func TestSanitizeOracleBackupPath(t *testing.T) {
	t.Parallel()
	// 跨平台合法绝对路径
	var validPath, absPrefix string
	if runtime.GOOS == "windows" {
		validPath = `C:\backup\oracle\rman.bak`
		absPrefix = `C:\backup\oracle`
	} else {
		validPath = "/backup/oracle/rman.bak"
		absPrefix = "/backup/oracle"
	}

	tests := []struct {
		name    string
		path    string
		wantErr bool
		wantSub string
	}{
		{"合法绝对路径", validPath, false, ""},
		{"空路径", "", true, "cannot be empty"},
		{"含单引号", validPath + "'", true, "single quotes"},
		{"相对路径", "backup/oracle.bak", true, "must be absolute"},
		// RMAN 元字符（使用平台相关绝对路径前缀确保通过绝对路径校验）
		{"含分号", absPrefix + ";rm -rf /", true, "RMAN meta characters"},
		{"含换行符", absPrefix + "\n.bak", true, "RMAN meta characters"},
		{"含回车符", absPrefix + "\r.bak", true, "RMAN meta characters"},
		{"含反引号", absPrefix + "`whoami`", true, "RMAN meta characters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := sanitizeOracleBackupPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("sanitizeOracleBackupPath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.wantSub != "" {
				if !strings.Contains(err.Error(), tt.wantSub) {
					t.Errorf("sanitizeOracleBackupPath(%q) 错误信息应包含 %q, got: %v", tt.path, tt.wantSub, err)
				}
			}
		})
	}
}

// dataDirSigTest 定义 validateDataDirSignature 测试用例
type dataDirSigTest struct {
	name      string
	dbType    string
	setupFunc func(dir string) error // 在临时目录中创建特征文件，nil 表示不创建
	wantErr   bool
	wantSub   string
}

func TestValidateDataDirSignature(t *testing.T) {
	t.Parallel()
	tests := []dataDirSigTest{
		{
			name: "MySQL 有 ibdata1",
			setupFunc: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "ibdata1"), []byte("data"), 0o644)
			},
			dbType: DBTypeMySQL,
		},
		{
			name: "MySQL 有 mysql 子目录",
			setupFunc: func(dir string) error {
				return os.Mkdir(filepath.Join(dir, "mysql"), 0o755)
			},
			dbType: DBTypeMySQL,
		},
		{
			name: "MySQL 无特征文件应报错", dbType: DBTypeMySQL,
			wantErr: true, wantSub: "MySQL data directory",
		},
		{
			name: "PostgreSQL 有 PG_VERSION",
			setupFunc: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "PG_VERSION"), []byte("13"), 0o644)
			},
			dbType: DBTypePostgreSQL,
		},
		{
			name: "PostgreSQL 无 PG_VERSION 应报错", dbType: DBTypePostgreSQL,
			wantErr: true, wantSub: "PostgreSQL data directory",
		},
		{
			name: "达梦 有 dm.ini",
			setupFunc: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "dm.ini"), []byte("[dm]"), 0o644)
			},
			dbType: DBTypeDameng,
		},
		{
			name: "达梦 无 dm.ini 应报错", dbType: DBTypeDameng,
			wantErr: true, wantSub: "Dameng data directory",
		},
		{
			name: "不支持的数据库类型应报错", dbType: "unsupported-type",
			wantErr: true, wantSub: "unsupported database type",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var dir string
			if tt.setupFunc != nil {
				dir = t.TempDir()
				if err := tt.setupFunc(dir); err != nil {
					t.Fatalf("创建测试特征文件失败: %v", err)
				}
			} else {
				dir = t.TempDir()
			}
			err := validateDataDirSignature(dir, tt.dbType)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDataDirSignature() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.wantSub != "" && !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("错误信息应包含 %q, got: %v", tt.wantSub, err)
			}
		})
	}
}
