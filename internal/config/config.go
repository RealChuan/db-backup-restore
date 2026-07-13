package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RealChuan/db-backup-restore/internal/logging"
)

// DBConfig 数据库连接配置
type DBConfig struct {
	Type      string            `json:"type"`        // 数据库类型：mysql, postgresql, oracle, mssql, dameng, kingbase, opengauss, gaussdb
	Host      string            `json:"host"`        // 主机地址
	Port      int               `json:"port"`        // 端口号
	User      string            `json:"user"`        // 用户名
	Password  string            `json:"password"`    // 密码
	Database  string            `json:"database"`    // 默认数据库
	SSLMode   string            `json:"ssl_mode"`    // SSL模式
	Extra     map[string]string `json:"extra"`       // 其他连接参数
	ExtraHelp string            `json:"_extra_help"` // 配置文件中的帮助注释，程序不使用
}

// DBType constants to avoid magic strings (mirrors backup package constants to prevent circular dependency).
const (
	dbTypeMySQL      = "mysql"
	dbTypePostgreSQL = "postgresql"
	dbTypeOracle     = "oracle"
	dbTypeMSSQL      = "mssql"
	dbTypeDameng     = "dameng"
	defaultHost      = "localhost"
)

// ErrDBConfigNotFound 表示指定数据库类型的配置不存在。
var ErrDBConfigNotFound = errors.New("database config not found")

// defaultPorts 各数据库默认端口映射
var defaultPorts = map[string]int{
	dbTypeMySQL:      3306,
	dbTypePostgreSQL: 5432,
	dbTypeOracle:     1521,
	dbTypeMSSQL:      1433,
	dbTypeDameng:     5236,
}

// DefaultPort 返回指定数据库类型的默认端口号。
func DefaultPort(dbType string) (int, bool) {
	p, ok := defaultPorts[dbType]
	return p, ok
}

// SetDefaults 为配置设置默认值
func (c *DBConfig) SetDefaults() {
	if c.Host == "" {
		c.Host = defaultHost
	}
	if c.Port == 0 {
		if defaultPort, exists := DefaultPort(c.Type); exists {
			c.Port = defaultPort
		}
	}
	if c.SSLMode == "" {
		c.SSLMode = "disable"
	}
	if c.Extra == nil {
		c.Extra = make(map[string]string)
	}
}

type LogConfig struct {
	Level         string `json:"level"`          // 日志级别: debug, info, warn, error
	Output        string `json:"output"`         // 输出位置: console, file, both
	Format        string `json:"format"`         // 输出格式: text, json
	LogFile       string `json:"log_file"`       // 日志文件路径
	AuditLogFile  string `json:"audit_log_file"` // 审计日志文件路径
	MaxFileSizeMB int    `json:"max_file_size"`  // 日志文件最大大小(MB)
	MaxBackups    int    `json:"max_backups"`    // 保留日志文件数量
	EnableColors  bool   `json:"enable_colors"`  // 是否启用颜色
	AddCaller     bool   `json:"add_caller"`     // 是否添加调用者信息
}

type Config struct {
	BaseBackupDir string              `json:"base_backup_dir"`
	Databases     map[string]DBConfig `json:"databases"`
	Log           *LogConfig          `json:"log"`
}

func LoadConfig(configPath string) (*Config, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cfg Config
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}

	cfg.setDefaults(configPath)

	// 校验各数据库的 extra 参数
	for name, dbCfg := range cfg.Databases {
		dbCfg.SetDefaults()
		if errs := dbCfg.ValidateExtra(); len(errs) > 0 {
			var errMsgs []string
			for _, e := range errs {
				errMsgs = append(errMsgs, e.Error())
			}
			return nil, fmt.Errorf("数据库 %q 配置校验失败: %s", name, strings.Join(errMsgs, "; "))
		}
		cfg.Databases[name] = dbCfg
	}

	// 关键配置项校验
	if cfg.BaseBackupDir == "" {
		return nil, errors.New("base_backup_dir 不能为空")
	}
	if len(cfg.Databases) == 0 {
		return nil, errors.New("databases 配置不能为空")
	}
	for name, dbCfg := range cfg.Databases {
		if dbCfg.Type == "" {
			return nil, fmt.Errorf("数据库 %q 缺少 type 配置", name)
		}
	}

	return &cfg, nil
}

func (cfg *Config) setDefaults(_ string) {
	if cfg.Log == nil {
		cfg.Log = &LogConfig{
			Level:         "info",
			Output:        "console",
			Format:        "text",
			LogFile:       "",
			AuditLogFile:  "",
			MaxFileSizeMB: 100,
			MaxBackups:    10,
			EnableColors:  true,
			AddCaller:     true,
		}
	}

	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Log.Output == "" {
		cfg.Log.Output = "console"
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = "text"
	}
	if cfg.Log.MaxFileSizeMB <= 0 {
		cfg.Log.MaxFileSizeMB = 100
	}
	if cfg.Log.MaxBackups <= 0 {
		cfg.Log.MaxBackups = 10
	}

	if cfg.Log.LogFile != "" && !filepath.IsAbs(cfg.Log.LogFile) {
		exePath, err := os.Executable()
		if err != nil {
			// fallback to working directory
			exePath, _ = os.Getwd()
		}
		if exePath != "" {
			exeDir := filepath.Dir(exePath)
			cfg.Log.LogFile = filepath.Join(exeDir, cfg.Log.LogFile)
		}
	}
	if cfg.Log.AuditLogFile != "" && !filepath.IsAbs(cfg.Log.AuditLogFile) {
		exePath, err := os.Executable()
		if err != nil {
			// fallback to working directory
			exePath, _ = os.Getwd()
		}
		if exePath != "" {
			exeDir := filepath.Dir(exePath)
			cfg.Log.AuditLogFile = filepath.Join(exeDir, cfg.Log.AuditLogFile)
		}
	}
}

func (cfg *Config) GetDBConfig(dbType string) (*DBConfig, error) {
	dbCfg, ok := cfg.Databases[dbType]
	if !ok {
		return nil, ErrDBConfigNotFound
	}

	return &dbCfg, nil
}

func (cfg *Config) GetLogConfig() *logging.Config {
	if cfg.Log == nil {
		return logging.DefaultConfig()
	}
	return &logging.Config{
		Level:         cfg.Log.Level,
		Output:        cfg.Log.Output,
		Format:        cfg.Log.Format,
		LogFile:       cfg.Log.LogFile,
		AuditLogFile:  cfg.Log.AuditLogFile,
		MaxFileSizeMB: cfg.Log.MaxFileSizeMB,
		MaxBackups:    cfg.Log.MaxBackups,
		EnableColors:  cfg.Log.EnableColors,
		AddCaller:     cfg.Log.AddCaller,
	}
}
