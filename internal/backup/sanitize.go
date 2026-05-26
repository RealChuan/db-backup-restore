package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// sanitizeDatabaseName 校验数据库名，防止 SQL 注入和误操作系统库。
// 采用通用宽松策略：只禁止真正危险的字符，支持 UTF-8 和国际化数据库名（如中文数据库名）。
// 安全设计原则：
//   - 禁止单引号(')：防止 SQL 字符串逃逸注入
//   - 禁止双引号(")：防止某些数据库的标识符引号注入
//   - 禁止分号(;)：防止多语句攻击
//   - 禁止反斜杠(\)：防止路径逃逸
//   - 禁止空字节(\x00)：防止字符串截断攻击
//   - 禁止换行符(\n\r)：防止命令注入
//   - 禁止方括号([])：防止 MSSQL 标识符注入
func sanitizeDatabaseName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("database name cannot be empty")
	}

	// 危险字符黑名单：这些字符可能被用于 SQL 注入或命令注入攻击
	dangerousChars := "'\";\\\x00\n\r[]"
	if strings.ContainsAny(name, dangerousChars) {
		return fmt.Errorf("database name contains dangerous characters: %q", name)
	}

	// 长度限制：128 字符（覆盖 MySQL 64、PostgreSQL 63、MSSQL 128、Oracle 128）
	// 使用最大支持长度作为统一限制，确保兼容性
	if len(name) > 128 {
		return fmt.Errorf("database name exceeds 128 characters: %d", len(name))
	}

	return nil
}

// sanitizeBackupPath 校验备份文件/目录路径，防止路径遍历和 SQL 字符串逃逸
func sanitizeBackupPath(path string, allowedExts ...string) (string, error) {
	if path == "" {
		return "", errors.New("backup path cannot be empty")
	}
	if strings.Contains(path, "'") {
		return "", errors.New("backup path cannot contain single quotes")
	}
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return "", errors.New("backup path must be absolute")
	}
	if len(allowedExts) > 0 {
		ext := strings.ToLower(filepath.Ext(path))
		valid := false
		for _, ae := range allowedExts {
			if ext == ae {
				valid = true
				break
			}
		}
		if !valid {
			return "", fmt.Errorf("backup path extension must be one of %v, got: %s", allowedExts, ext)
		}
	}
	return path, nil
}

// sanitizePositiveInt 强制字符串为正整数，拒绝任何非数字输入
func sanitizePositiveInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("value cannot be empty")
	}
	val, err := strconv.Atoi(s)
	if err != nil || val <= 0 {
		return 0, fmt.Errorf("value must be a positive integer, got: %q", s)
	}
	return val, nil
}

// sanitizeDateLiteral 校验日期字符串，仅允许 YYYY-MM-DD HH:MM:SS 格式
func sanitizeDateLiteral(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("date literal cannot be empty")
	}
	if strings.Contains(s, "'") {
		return "", errors.New("date literal cannot contain single quotes")
	}
	if _, err := time.Parse("2006-01-02 15:04:05", s); err != nil {
		return "", fmt.Errorf("date literal must match format 'YYYY-MM-DD HH:MM:SS', got: %q", s)
	}
	return s, nil
}

// escapeOracleRMANString 对 RMAN 脚本中的单引号字符串进行转义
// Oracle/RMAN 中单引号转义规则：两个连续单引号表示一个字面量单引号
func escapeOracleRMANString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// sanitizeOracleBackupID 校验 Oracle 备份集 ID，仅允许安全字符
func sanitizeOracleBackupID(id string) error {
	if id == "" {
		return errors.New("backup ID cannot be empty")
	}
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_\-]+$`, id)
	if !matched {
		return fmt.Errorf("oracle backup ID contains illegal characters: %q", id)
	}
	return nil
}

// sanitizeOracleBackupPath 校验 Oracle 备份路径，额外拒绝 RMAN 元字符
func sanitizeOracleBackupPath(path string) error {
	if path == "" {
		return errors.New("backup path cannot be empty")
	}
	if _, err := sanitizeBackupPath(path); err != nil {
		return err
	}
	if strings.ContainsAny(path, ";\n\r`") {
		return fmt.Errorf("backup path contains RMAN meta characters: %q", path)
	}
	return nil
}

// validateDataDir 对物理还原的目标数据目录进行严格校验。
// 安全设计原则：
//  1. 路径必须是绝对路径，防止相对路径攻击（如 ../../etc）
//  2. 禁止根目录，防止误删整个系统
//  3. 禁止系统目录，防止破坏系统文件
//  4. 验证数据库特征文件，确保是有效的数据库数据目录
//
// 物理还原是高风险操作，此函数用于防止以下安全风险：
//   - 路径遍历攻击：通过构造恶意路径删除系统文件
//   - 误操作：用户错误配置导致删除重要数据
//   - 数据丢失：还原到错误目录导致数据覆盖
func validateDataDir(datadir string, dbType string) error {
	if datadir == "" {
		return errors.New("DATA_DIR cannot be empty")
	}

	cleanPath := filepath.Clean(datadir)

	if err := validateDataDirPath(cleanPath); err != nil {
		return err
	}

	return validateDataDirSignature(cleanPath, dbType)
}

// validateDataDirPath 校验路径安全性和合法性
func validateDataDirPath(cleanPath string) error {
	if !filepath.IsAbs(cleanPath) {
		return errors.New("DATA_DIR must be an absolute path")
	}

	if cleanPath == "/" || cleanPath == "\\" ||
		(len(cleanPath) == 3 && strings.HasSuffix(cleanPath, ":\\")) {
		return errors.New("DATA_DIR cannot be root directory")
	}

	forbiddenPrefixes := []string{
		"/etc", "/usr", "/bin", "/sbin", "/boot", "/dev", "/proc", "/sys",
		"C:\\Windows", "C:\\Program Files", "C:\\ProgramData",
	}
	lowerPath := strings.ToLower(cleanPath)
	for _, prefix := range forbiddenPrefixes {
		if strings.HasPrefix(lowerPath, strings.ToLower(prefix)) {
			return fmt.Errorf("DATA_DIR cannot be within system directory: %s", prefix)
		}
	}

	return nil
}

// validateDataDirSignature 验证数据库特征文件
func validateDataDirSignature(cleanPath string, dbType string) error {
	switch dbType {
	case DBTypeMySQL:
		hasIBData := false
		hasMySQLDir := false
		if _, err := os.Stat(filepath.Join(cleanPath, "ibdata1")); err == nil {
			hasIBData = true
		}
		if _, err := os.Stat(filepath.Join(cleanPath, "mysql")); err == nil {
			hasMySQLDir = true
		}
		if !hasIBData && !hasMySQLDir {
			return fmt.Errorf("DATA_DIR %s does not appear to be a valid MySQL data directory (missing ibdata1 or mysql/ dir)", cleanPath)
		}
	case DBTypePostgreSQL:
		if _, err := os.Stat(filepath.Join(cleanPath, "PG_VERSION")); os.IsNotExist(err) {
			return fmt.Errorf("DATA_DIR %s does not appear to be a valid PostgreSQL data directory (missing PG_VERSION)", cleanPath)
		}
	default:
		return fmt.Errorf("unsupported database type for data directory validation: %s", dbType)
	}
	return nil
}

// mustBeUnderBackupDir 校验路径必须在指定的备份目录下（防止信息泄露）
func mustBeUnderBackupDir(path string, backupDir string) error {
	if backupDir == "" {
		return errors.New("backup directory not configured")
	}
	cleanPath := filepath.Clean(path)
	cleanBackupDir := filepath.Clean(backupDir)

	if !strings.HasPrefix(cleanPath, cleanBackupDir+string(os.PathSeparator)) && cleanPath != cleanBackupDir {
		return fmt.Errorf("path %s is not within backup directory %s", cleanPath, cleanBackupDir)
	}
	return nil
}
