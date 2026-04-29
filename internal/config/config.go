package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"db-backup-restore/internal/backup"
	"db-backup-restore/pkg/utils"
)

type DatabaseConfig struct {
	Type     string            `json:"type"`
	Host     string            `json:"host"`
	Port     int               `json:"port"`
	User     string            `json:"user"`
	Password string            `json:"password"`
	Database string            `json:"database"`
	SSLMode  string            `json:"ssl_mode"`
	Extra    map[string]string `json:"extra"`
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
	BaseBackupDir string                    `json:"base_backup_dir"`
	Databases     map[string]DatabaseConfig `json:"databases"`
	Log           *LogConfig                `json:"log"`
}

func LoadConfig(configPath string) (*Config, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cfg Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}

	cfg.setDefaults(configPath)

	return &cfg, nil
}

func (cfg *Config) setDefaults(configPath string) {
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
		exePath, _ := os.Executable()
		exeDir := filepath.Dir(exePath)
		cfg.Log.LogFile = filepath.Join(exeDir, cfg.Log.LogFile)
	}
	if cfg.Log.AuditLogFile != "" && !filepath.IsAbs(cfg.Log.AuditLogFile) {
		exePath, _ := os.Executable()
		exeDir := filepath.Dir(exePath)
		cfg.Log.AuditLogFile = filepath.Join(exeDir, cfg.Log.AuditLogFile)
	}
}

func (cfg *Config) GetDBConfig(dbType string) (*backup.DBConfig, error) {
	dbCfg, ok := cfg.Databases[dbType]
	if !ok {
		return nil, os.ErrNotExist
	}

	return &backup.DBConfig{
		Type:     dbCfg.Type,
		Host:     dbCfg.Host,
		Port:     dbCfg.Port,
		User:     dbCfg.User,
		Password: dbCfg.Password,
		Database: dbCfg.Database,
		SSLMode:  dbCfg.SSLMode,
		Extra:    dbCfg.Extra,
	}, nil
}

func (cfg *Config) GetLogConfig() *utils.LogConfig {
	if cfg.Log == nil {
		return utils.NewLogConfig()
	}
	return &utils.LogConfig{
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
