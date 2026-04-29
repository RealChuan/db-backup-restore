package utils

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

var Logger *logrus.Logger
var AuditLogger *logrus.Logger

var logLevel sync.RWMutex
var currentLevel logrus.Level

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

func NewLogConfig() *LogConfig {
	return &LogConfig{
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

type rotatingWriter struct {
	mu           sync.Mutex
	filePath     string
	maxSizeBytes int64
	maxBackups   int
	currentSize  int64
	file         *os.File
}

func newRotatingWriter(filePath string, maxSizeMB, maxBackups int) (*rotatingWriter, error) {
	rw := &rotatingWriter{
		filePath:     filePath,
		maxSizeBytes: int64(maxSizeMB) * 1024 * 1024,
		maxBackups:   maxBackups,
		currentSize:  0,
	}

	if err := ensureDirExists(filePath); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	rw.file = file

	if info, err := file.Stat(); err == nil {
		rw.currentSize = info.Size()
	}

	return rw, nil
}

func (rw *rotatingWriter) Write(p []byte) (n int, err error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	if rw.currentSize+int64(len(p)) > rw.maxSizeBytes {
		if err := rw.rotate(); err != nil {
			return 0, err
		}
	}

	n, err = rw.file.Write(p)
	if err == nil {
		rw.currentSize += int64(n)
	}
	return n, err
}

func (rw *rotatingWriter) rotate() error {
	if err := rw.file.Close(); err != nil {
		return err
	}

	ext := filepath.Ext(rw.filePath)
	base := rw.filePath[:len(rw.filePath)-len(ext)]

	for i := rw.maxBackups - 1; i > 0; i-- {
		src := fmt.Sprintf("%s.%d%s", base, i, ext)
		dst := fmt.Sprintf("%s.%d%s", base, i+1, ext)
		os.Rename(src, dst)
	}

	if rw.maxBackups > 0 {
		os.Rename(rw.filePath, fmt.Sprintf("%s.1%s", base, ext))
	}

	file, err := os.OpenFile(rw.filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	rw.file = file
	rw.currentSize = 0
	return nil
}

func (rw *rotatingWriter) Close() error {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	return rw.file.Close()
}

type CustomFormatter struct {
	logrus.TextFormatter
	EnableColors bool
	AddCaller    bool
}

func (f *CustomFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	var levelColor string
	var resetColor = "\x1b[0m"

	if f.EnableColors {
		switch entry.Level {
		case logrus.DebugLevel:
			levelColor = "\x1b[36m"
		case logrus.InfoLevel:
			levelColor = "\x1b[32m"
		case logrus.WarnLevel:
			levelColor = "\x1b[33m"
		case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
			levelColor = "\x1b[31m"
		default:
			levelColor = "\x1b[37m"
		}
	}

	cmdOutput, hasCmdOutput := entry.Data["cmd_output"]
	traceID, hasTraceID := entry.Data["trace_id"]
	caller, hasCaller := entry.Data["caller"]

	if hasCmdOutput {
		delete(entry.Data, "cmd_output")
	}
	if hasTraceID {
		delete(entry.Data, "trace_id")
	}
	if hasCaller {
		delete(entry.Data, "caller")
	}

	timestamp := entry.Time.Format("2006-01-02 15:04:05.000")
	levelStr := strings.ToUpper(entry.Level.String())

	var callerStr string
	if f.AddCaller && hasCaller {
		callerStr = fmt.Sprintf("[%s]", caller)
	}

	var formatted string
	if hasCmdOutput {
		formatted = fmt.Sprintf("%s [%s]%s %s\n%s",
			timestamp,
			levelStr,
			callerStr,
			entry.Message,
			cmdOutput,
		)
	} else {
		tracePrefix := ""
		if hasTraceID {
			tracePrefix = fmt.Sprintf("[%s] ", traceID)
		}
		if f.EnableColors {
			formatted = fmt.Sprintf("%s %s%s%s%s %s\n",
				timestamp,
				levelColor,
				levelStr,
				resetColor,
				callerStr,
				tracePrefix+entry.Message,
			)
		} else {
			formatted = fmt.Sprintf("%s %s%s %s\n",
				timestamp,
				levelStr,
				callerStr,
				tracePrefix+entry.Message,
			)
		}
	}

	return []byte(formatted), nil
}

type JSONFormatter struct {
	logrus.JSONFormatter
}

func (f *JSONFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	if entry.HasCaller() {
		entry.Data["caller"] = fmt.Sprintf("%s:%d", filepath.Base(entry.Caller.File), entry.Caller.Line)
		entry.Data["function"] = entry.Caller.Function
	}

	traceID, hasTraceID := entry.Data["trace_id"]
	if hasTraceID {
		entry.Data["trace_id"] = traceID
	}

	return f.JSONFormatter.Format(entry)
}

func ensureDirExists(filePath string) error {
	dir := filepath.Dir(filePath)
	if dir == "." || dir == string(filepath.Separator) {
		return nil
	}
	return os.MkdirAll(dir, 0755)
}

func InitLogger(config *LogConfig) error {
	Logger = logrus.New()

	level, err := logrus.ParseLevel(config.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	Logger.SetLevel(level)
	currentLevel = level

	var outputs []io.Writer
	if config.Output == "console" || config.Output == "both" {
		outputs = append(outputs, os.Stdout)
	}
	if config.Output == "file" || config.Output == "both" {
		if config.LogFile != "" {
			if err := ensureDirExists(config.LogFile); err != nil {
				return fmt.Errorf("无法创建日志目录: %w", err)
			}
			if config.MaxFileSizeMB > 0 && config.MaxBackups > 0 {
				rw, err := newRotatingWriter(config.LogFile, config.MaxFileSizeMB, config.MaxBackups)
				if err != nil {
					return fmt.Errorf("无法创建轮转日志写入器: %w", err)
				}
				outputs = append(outputs, rw)
			} else {
				file, err := os.OpenFile(config.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
				if err != nil {
					return fmt.Errorf("无法打开日志文件: %w", err)
				}
				outputs = append(outputs, file)
			}
		}
	}

	if len(outputs) > 0 {
		Logger.SetOutput(io.MultiWriter(outputs...))
	}

	if config.Format == "json" {
		Logger.SetFormatter(&JSONFormatter{
			JSONFormatter: logrus.JSONFormatter{
				TimestampFormat: "2006-01-02 15:04:05.000",
			},
		})
	} else {
		Logger.SetFormatter(&CustomFormatter{
			TextFormatter: logrus.TextFormatter{
				FullTimestamp:   true,
				TimestampFormat: "2006-01-02 15:04:05.000",
				DisableQuote:    true,
				ForceColors:     config.EnableColors,
			},
			EnableColors: config.EnableColors,
			AddCaller:    config.AddCaller,
		})
	}

	Logger.SetReportCaller(false)

	if config.AuditLogFile != "" {
		AuditLogger = logrus.New()
		AuditLogger.SetLevel(logrus.InfoLevel)

		if err := ensureDirExists(config.AuditLogFile); err != nil {
			return fmt.Errorf("无法创建审计日志目录: %w", err)
		}

		var auditOutput io.Writer
		if config.MaxFileSizeMB > 0 && config.MaxBackups > 0 {
			rw, err := newRotatingWriter(config.AuditLogFile, config.MaxFileSizeMB, config.MaxBackups)
			if err != nil {
				return fmt.Errorf("无法创建审计日志轮转写入器: %w", err)
			}
			auditOutput = rw
		} else {
			file, err := os.OpenFile(config.AuditLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				return fmt.Errorf("无法打开审计日志文件: %w", err)
			}
			auditOutput = file
		}

		AuditLogger.SetOutput(auditOutput)
		AuditLogger.SetFormatter(&JSONFormatter{
			JSONFormatter: logrus.JSONFormatter{
				TimestampFormat: "2006-01-02 15:04:05.000",
			},
		})
	}

	return nil
}

func SetLogLevel(level string) error {
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		return err
	}
	logLevel.Lock()
	currentLevel = lvl
	logLevel.Unlock()
	if Logger != nil {
		Logger.SetLevel(lvl)
	}
	return nil
}

func GetLogLevel() string {
	logLevel.RLock()
	defer logLevel.RUnlock()
	return currentLevel.String()
}

var traceIDKey string
var traceIDOnce sync.Once

func SetTraceID(traceID string) {
	traceIDOnce.Do(func() {
		traceIDKey = traceID
	})
}

func GetTraceID() string {
	return traceIDKey
}

func ClearTraceID() {
	traceIDKey = ""
}

func generateTraceID() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + strconv.Itoa(os.Getpid())
}

func InitTraceID() {
	traceIDKey = generateTraceID()
}

func FormatCommandOutput(cmd string, output string, isError bool) string {
	var buf bytes.Buffer
	lines := strings.Split(output, "\n")

	filteredLines := make([]string, 0)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.Contains(trimmed, "==========") {
			filteredLines = append(filteredLines, trimmed)
		}
	}

	if len(filteredLines) > 0 {
		header := "┌─────────────────────────────────────────────────────────────"
		footer := "└─────────────────────────────────────────────────────────────"

		buf.WriteString(header + "\n")
		buf.WriteString("│ COMMAND: " + cmd + "\n")
		buf.WriteString("│─────────────────────────────────────────────────────────────\n")

		if len(filteredLines) <= 30 {
			for _, line := range filteredLines {
				buf.WriteString("│ " + line + "\n")
			}
		} else {
			for i := 0; i < 10; i++ {
				buf.WriteString("│ " + filteredLines[i] + "\n")
			}
			buf.WriteString("│ ... (省略 " + fmt.Sprintf("%d", len(filteredLines)-20) + " 行)\n")
			for i := len(filteredLines) - 10; i < len(filteredLines); i++ {
				buf.WriteString("│ " + filteredLines[i] + "\n")
			}
		}

		buf.WriteString(footer + "\n")
	}

	return buf.String()
}

func LogCommand(cmd string, output string, isError bool) {
	formattedOutput := FormatCommandOutput(cmd, output, isError)

	if isError {
		Logger.WithField("cmd_output", formattedOutput).Error("命令执行失败")
	} else {
		Logger.WithField("cmd_output", formattedOutput).Debug("命令执行输出")
	}
}

func LogCommandInfo(cmd string) {
	Logger.Info(fmt.Sprintf("[命令执行] %s", cmd))
}

func getCallerInfo() *runtime.Frame {
	pc := make([]uintptr, 1)
	n := runtime.Callers(3, pc)
	if n == 0 {
		return nil
	}
	frame, _ := runtime.CallersFrames(pc).Next()
	return &frame
}

func Info(args ...interface{}) {
	if Logger == nil {
		fmt.Println(args...)
		return
	}
	traceID := GetTraceID()
	caller := getCallerInfo()
	if caller != nil {
		entry := Logger.WithField("caller", fmt.Sprintf("%s:%d", filepath.Base(caller.File), caller.Line))
		if traceID != "" {
			entry = entry.WithField("trace_id", traceID)
		}
		entry.Info(args...)
	} else {
		if traceID != "" {
			Logger.WithField("trace_id", traceID).Info(args...)
		} else {
			Logger.Info(args...)
		}
	}
}

func Infof(format string, args ...interface{}) {
	if Logger == nil {
		fmt.Printf(format+"\n", args...)
		return
	}
	traceID := GetTraceID()
	caller := getCallerInfo()
	message := fmt.Sprintf(format, args...)
	if caller != nil {
		entry := Logger.WithField("caller", fmt.Sprintf("%s:%d", filepath.Base(caller.File), caller.Line))
		if traceID != "" {
			entry = entry.WithField("trace_id", traceID)
		}
		entry.Info(message)
	} else {
		if traceID != "" {
			Logger.WithField("trace_id", traceID).Info(message)
		} else {
			Logger.Infof("%s", message)
		}
	}
}

func Warn(args ...interface{}) {
	if Logger == nil {
		fmt.Println(args...)
		return
	}
	traceID := GetTraceID()
	caller := getCallerInfo()
	if caller != nil {
		entry := Logger.WithField("caller", fmt.Sprintf("%s:%d", filepath.Base(caller.File), caller.Line))
		if traceID != "" {
			entry = entry.WithField("trace_id", traceID)
		}
		entry.Warn(args...)
	} else {
		if traceID != "" {
			Logger.WithField("trace_id", traceID).Warn(args...)
		} else {
			Logger.Warn(args...)
		}
	}
}

func Warnf(format string, args ...interface{}) {
	if Logger == nil {
		fmt.Printf(format+"\n", args...)
		return
	}
	traceID := GetTraceID()
	caller := getCallerInfo()
	message := fmt.Sprintf(format, args...)
	if caller != nil {
		entry := Logger.WithField("caller", fmt.Sprintf("%s:%d", filepath.Base(caller.File), caller.Line))
		if traceID != "" {
			entry = entry.WithField("trace_id", traceID)
		}
		entry.Warn(message)
	} else {
		if traceID != "" {
			Logger.WithField("trace_id", traceID).Warn(message)
		} else {
			Logger.Warnf("%s", message)
		}
	}
}

func Error(args ...interface{}) {
	if Logger == nil {
		fmt.Println(args...)
		return
	}
	traceID := GetTraceID()
	caller := getCallerInfo()
	if caller != nil {
		entry := Logger.WithField("caller", fmt.Sprintf("%s:%d", filepath.Base(caller.File), caller.Line))
		if traceID != "" {
			entry = entry.WithField("trace_id", traceID)
		}
		entry.Error(args...)
	} else {
		if traceID != "" {
			Logger.WithField("trace_id", traceID).Error(args...)
		} else {
			Logger.Error(args...)
		}
	}
}

func Errorf(format string, args ...interface{}) {
	if Logger == nil {
		fmt.Printf(format+"\n", args...)
		return
	}
	traceID := GetTraceID()
	caller := getCallerInfo()
	message := fmt.Sprintf(format, args...)
	if caller != nil {
		entry := Logger.WithField("caller", fmt.Sprintf("%s:%d", filepath.Base(caller.File), caller.Line))
		if traceID != "" {
			entry = entry.WithField("trace_id", traceID)
		}
		entry.Error(message)
	} else {
		if traceID != "" {
			Logger.WithField("trace_id", traceID).Error(message)
		} else {
			Logger.Errorf("%s", message)
		}
	}
}

func Fatal(args ...interface{}) {
	if Logger == nil {
		fmt.Println(args...)
		os.Exit(1)
		return
	}
	traceID := GetTraceID()
	caller := getCallerInfo()
	if caller != nil {
		entry := Logger.WithField("caller", fmt.Sprintf("%s:%d", filepath.Base(caller.File), caller.Line))
		if traceID != "" {
			entry = entry.WithField("trace_id", traceID)
		}
		entry.Fatal(args...)
	} else {
		if traceID != "" {
			Logger.WithField("trace_id", traceID).Fatal(args...)
		} else {
			Logger.Fatal(args...)
		}
	}
	os.Exit(1)
}

func Fatalf(format string, args ...interface{}) {
	if Logger == nil {
		fmt.Printf(format+"\n", args...)
		os.Exit(1)
		return
	}
	traceID := GetTraceID()
	caller := getCallerInfo()
	message := fmt.Sprintf(format, args...)
	if caller != nil {
		entry := Logger.WithField("caller", fmt.Sprintf("%s:%d", filepath.Base(caller.File), caller.Line))
		if traceID != "" {
			entry = entry.WithField("trace_id", traceID)
		}
		entry.Fatal(message)
	} else {
		if traceID != "" {
			Logger.WithField("trace_id", traceID).Fatal(message)
		} else {
			Logger.Fatalf("%s", message)
		}
	}
	os.Exit(1)
}

func Debug(args ...interface{}) {
	if Logger == nil {
		fmt.Println(args...)
		return
	}
	traceID := GetTraceID()
	caller := getCallerInfo()
	if caller != nil {
		entry := Logger.WithField("caller", fmt.Sprintf("%s:%d", filepath.Base(caller.File), caller.Line))
		if traceID != "" {
			entry = entry.WithField("trace_id", traceID)
		}
		entry.Debug(args...)
	} else {
		if traceID != "" {
			Logger.WithField("trace_id", traceID).Debug(args...)
		} else {
			Logger.Debug(args...)
		}
	}
}

func Debugf(format string, args ...interface{}) {
	if Logger == nil {
		fmt.Printf(format+"\n", args...)
		return
	}
	traceID := GetTraceID()
	caller := getCallerInfo()
	message := fmt.Sprintf(format, args...)
	if caller != nil {
		entry := Logger.WithField("caller", fmt.Sprintf("%s:%d", filepath.Base(caller.File), caller.Line))
		if traceID != "" {
			entry = entry.WithField("trace_id", traceID)
		}
		entry.Debug(message)
	} else {
		if traceID != "" {
			Logger.WithField("trace_id", traceID).Debug(message)
		} else {
			Logger.Debugf("%s", message)
		}
	}
}

func AuditLog(action string, dbType string, status string, details ...string) {
	if AuditLogger == nil {
		return
	}

	entry := AuditLogger.WithFields(logrus.Fields{
		"timestamp": time.Now().Format("2006-01-02 15:04:05.000"),
		"action":    action,
		"db_type":   dbType,
		"status":    status,
	})

	if len(details) > 0 {
		entry = entry.WithField("details", strings.Join(details, ", "))
	}

	entry.Info()
}
