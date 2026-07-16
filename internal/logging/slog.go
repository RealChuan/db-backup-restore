// Package logging provides a slog-based logging system that supports console
// and file output, log rotation, and audit logging.
package logging

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Config holds the logging configuration.
type Config struct {
	Level         string // 日志级别: debug, info, warn, error
	Output        string // 输出位置: console, file, both
	Format        string // 输出格式: text, json
	LogFile       string // 日志文件路径
	AuditLogFile  string // 审计日志文件路径
	MaxFileSizeMB int    // 日志文件最大大小(MB)
	MaxBackups    int    // 保留日志文件数量
	EnableColors  bool   // 是否启用颜色
	AddCaller     bool   // 是否添加调用者信息
}

// Config 默认值常量，避免 goconst 重复字符串告警
const (
	defaultLevel  = "info"
	defaultOutput = "console"
	defaultFormat = "text"
)

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Level:         defaultLevel,
		Output:        defaultOutput,
		Format:        defaultFormat,
		LogFile:       "",
		AuditLogFile:  "",
		MaxFileSizeMB: 100,
		MaxBackups:    10,
		EnableColors:  true,
		AddCaller:     true,
	}
}

var (
	globalLogger *slog.Logger
	auditLogger  *slog.Logger
	loggerMu     sync.RWMutex
	closers      []io.Closer
)

// L returns the global logger. If Init has not been called, it returns a
// default slog logger writing to stdout.
func L() *slog.Logger {
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	if globalLogger == nil {
		return slog.Default()
	}
	return globalLogger
}

// Audit returns the audit logger. If no audit logger was configured, it
// returns nil.
func Audit() *slog.Logger {
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	return auditLogger
}

// Close gracefully closes all registered io.Closer resources (log files, etc.)
// created during Init. It should be called before program exit.
func Close() {
	loggerMu.Lock()
	defer loggerMu.Unlock()
	for _, c := range closers {
		_ = c.Close()
	}
	closers = nil
}

// Init initialises the logging system according to the supplied Config.
func Init(cfg *Config) error {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v; falling back to info\n", err)
		level = slog.LevelInfo
	}

	// Track all created closers so we can clean up on failure.
	var createdClosers []io.Closer

	var handlers []slog.Handler

	if cfg.Output == "console" || cfg.Output == "both" {
		h := newConsoleHandler(cfg, os.Stdout, level)
		handlers = append(handlers, h)
	}

	if (cfg.Output == "file" || cfg.Output == "both") && cfg.LogFile != "" {
		w, err := newWriterForFile(cfg.LogFile, cfg.MaxFileSizeMB, cfg.MaxBackups)
		if err != nil {
			return fmt.Errorf("无法创建日志写入器: %w", err)
		}
		if c, ok := w.(io.Closer); ok {
			createdClosers = append(createdClosers, c)
		}
		h := newFileHandler(cfg, w, level)
		handlers = append(handlers, h)
	}

	var handler slog.Handler
	switch len(handlers) {
	case 0:
		handler = newConsoleHandler(cfg, os.Stdout, level)
	case 1:
		handler = handlers[0]
	default:
		handler = &multiHandler{handlers: handlers}
	}

	loggerMu.Lock()
	globalLogger = slog.New(handler)
	closers = createdClosers
	loggerMu.Unlock()

	slog.SetDefault(globalLogger)

	if cfg.AuditLogFile != "" {
		w, err := newWriterForFile(cfg.AuditLogFile, cfg.MaxFileSizeMB, cfg.MaxBackups)
		if err != nil {
			// Clean up already-created main log writers before returning.
			for _, c := range createdClosers {
				_ = c.Close()
			}
			loggerMu.Lock()
			closers = nil
			loggerMu.Unlock()
			return fmt.Errorf("无法创建审计日志写入器: %w", err)
		}
		if c, ok := w.(io.Closer); ok {
			createdClosers = append(createdClosers, c)
		}
		auditHandler := slog.NewJSONHandler(w, &slog.HandlerOptions{
			Level:       slog.LevelInfo,
			ReplaceAttr: timestampReplacer,
		})
		loggerMu.Lock()
		auditLogger = slog.New(auditHandler)
		closers = createdClosers
		loggerMu.Unlock()
	}

	return nil
}

// ---------------------------------------------------------------------------
// multiHandler – dispatches to multiple slog handlers
// ---------------------------------------------------------------------------

type multiHandler struct {
	handlers []slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		newHandlers[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: newHandlers}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		newHandlers[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: newHandlers}
}

// ---------------------------------------------------------------------------
// rotatingWriter – file rotation support
// ---------------------------------------------------------------------------

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

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
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
	ext := filepath.Ext(rw.filePath)
	base := rw.filePath[:len(rw.filePath)-len(ext)]

	for i := rw.maxBackups - 1; i > 0; i-- {
		src := fmt.Sprintf("%s.%d%s", base, i, ext)
		dst := fmt.Sprintf("%s.%d%s", base, i+1, ext)
		if err := os.Rename(src, dst); err != nil {
			if !os.IsNotExist(err) {
				if removeErr := os.Remove(dst); removeErr != nil && !os.IsNotExist(removeErr) {
					slog.Warn("rotate: failed to remove destination before rename retry",
						"src", src, "dst", dst, "error", removeErr)
				}
				if renameErr := os.Rename(src, dst); renameErr != nil {
					slog.Warn("rotate: rename retry failed",
						"src", src, "dst", dst, "error", renameErr)
				}
			}
		}
	}

	if rw.maxBackups > 0 {
		backupPath := fmt.Sprintf("%s.1%s", base, ext)
		if err := os.Rename(rw.filePath, backupPath); err != nil {
			if copyErr := copyFile(rw.filePath, backupPath); copyErr == nil {
				_ = os.Remove(rw.filePath)
			} else {
				slog.Warn("rotate: failed to copy current log to backup",
					"src", rw.filePath, "dst", backupPath, "error", copyErr)
			}
		}
	}

	// Open the new file first; if this fails the old file remains usable.
	newFile, err := os.OpenFile(rw.filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		// Do not close old file — keep it writable.
		return err
	}
	rw.file.Close()
	rw.file = newFile
	rw.currentSize = 0
	return nil
}

func (rw *rotatingWriter) Close() error {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	return rw.file.Close()
}

// ---------------------------------------------------------------------------
// Helper functions for file handling
// ---------------------------------------------------------------------------

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	return destFile.Sync()
}

func ensureDirExists(filePath string) error {
	dir := filepath.Dir(filePath)
	if dir == "." || dir == string(filepath.Separator) {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func newWriterForFile(path string, maxSizeMB, maxBackups int) (io.Writer, error) {
	if err := ensureDirExists(path); err != nil {
		return nil, fmt.Errorf("无法创建日志目录: %w", err)
	}
	if maxSizeMB > 0 && maxBackups > 0 {
		return newRotatingWriter(path, maxSizeMB, maxBackups)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("无法打开日志文件: %w", err)
	}
	return file, nil
}

// ---------------------------------------------------------------------------
// Console handler with color support
// ---------------------------------------------------------------------------

type consoleHandler struct {
	w            io.Writer
	level        slog.Level
	enableColors bool
	addCaller    bool
	attrs        []slog.Attr
	groups       []string
}

func newConsoleHandler(cfg *Config, w io.Writer, level slog.Level) *consoleHandler {
	return &consoleHandler{
		w:            w,
		level:        level,
		enableColors: cfg.EnableColors,
		addCaller:    cfg.AddCaller,
	}
}

func (h *consoleHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

//nolint:gocyclo // 复杂度来自属性分发和条件格式化，重构会降低可读性
func (h *consoleHandler) Handle(_ context.Context, r slog.Record) error {
	timestamp := r.Time.Format("2006-01-02 15:04:05.000")
	levelStr := levelString(r.Level)

	var callerStr string
	if h.addCaller {
		if r.PC != 0 {
			frame, _ := runtime.CallersFrames([]uintptr{r.PC}).Next()
			if frame.File != "" {
				callerStr = fmt.Sprintf("[%s:%d]", filepath.Base(frame.File), frame.Line)
			}
		}
	}

	var traceID string
	var cmdOutput string
	otherAttrSet := make(map[string]string)
	attrOrder := make([]string, 0)

	// Process h.attrs first (handler defaults), then r.Attrs() (record overrides).
	for _, a := range h.attrs {
		switch a.Key {
		case "trace_id":
			traceID = a.Value.String()
		case "cmd_output":
			cmdOutput = a.Value.String()
		default:
			if _, exists := otherAttrSet[a.Key]; !exists {
				attrOrder = append(attrOrder, a.Key)
			}
			otherAttrSet[a.Key] = fmt.Sprintf("%v", a.Value)
		}
	}

	r.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "trace_id":
			traceID = a.Value.String()
		case "cmd_output":
			cmdOutput = a.Value.String()
		default:
			if _, exists := otherAttrSet[a.Key]; !exists {
				attrOrder = append(attrOrder, a.Key)
			}
			otherAttrSet[a.Key] = fmt.Sprintf("%v", a.Value)
		}
		return true
	})

	var otherAttrs []string
	for _, key := range attrOrder {
		otherAttrs = append(otherAttrs, fmt.Sprintf("%s=%s", key, otherAttrSet[key]))
	}

	var buf bytes.Buffer

	if cmdOutput != "" {
		fmt.Fprintf(&buf, "%s [%s]%s %s\n%s",
			timestamp, levelStr, callerStr, r.Message, cmdOutput)
	} else {
		tracePrefix := ""
		if traceID != "" {
			tracePrefix = fmt.Sprintf("[%s] ", traceID)
		}
		if h.enableColors {
			color := levelColor(r.Level)
			reset := "\x1b[0m"
			fmt.Fprintf(&buf, "%s %s%s%s%s %s\n",
				timestamp, color, levelStr, reset, callerStr, tracePrefix+r.Message)
		} else {
			fmt.Fprintf(&buf, "%s %s%s %s\n",
				timestamp, levelStr, callerStr, tracePrefix+r.Message)
		}
	}

	if len(otherAttrs) > 0 {
		buf.WriteString("  " + strings.Join(otherAttrs, " ") + "\n")
	}

	_, err := h.w.Write(buf.Bytes())
	return err
}

func (h *consoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &consoleHandler{
		w:            h.w,
		level:        h.level,
		enableColors: h.enableColors,
		addCaller:    h.addCaller,
		attrs:        newAttrs,
		groups:       h.groups,
	}
}

// WithGroup returns a new handler with the given group appended. Note:
// consoleHandler does not support group semantics — the group name is
// recorded but ignored during formatting.
func (h *consoleHandler) WithGroup(name string) slog.Handler {
	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name
	return &consoleHandler{
		w:            h.w,
		level:        h.level,
		enableColors: h.enableColors,
		addCaller:    h.addCaller,
		attrs:        h.attrs,
		groups:       newGroups,
	}
}

// ---------------------------------------------------------------------------
// File handler (text or JSON)
// ---------------------------------------------------------------------------

func newFileHandler(cfg *Config, w io.Writer, level slog.Level) slog.Handler {
	opts := &slog.HandlerOptions{
		Level:       level,
		AddSource:   cfg.AddCaller,
		ReplaceAttr: timestampReplacer,
	}
	if cfg.Format == "json" {
		return slog.NewJSONHandler(w, opts)
	}
	return slog.NewTextHandler(w, opts)
}

// ---------------------------------------------------------------------------
// Level and color helpers
// ---------------------------------------------------------------------------

// 日志级别字符串常量，避免 goconst 重复字符串告警
const (
	levelStrDebug = "DEBUG"
	levelStrInfo  = "INFO"
	levelStrWarn  = "WARN"
	levelStrError = "ERROR"
)

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case defaultLevel:
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level: %s", s)
	}
}

func levelString(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return levelStrError
	case l >= slog.LevelWarn:
		return levelStrWarn
	case l >= slog.LevelInfo:
		return levelStrInfo
	default:
		return levelStrDebug
	}
}

func levelColor(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "\x1b[31m"
	case l >= slog.LevelWarn:
		return "\x1b[33m"
	case l >= slog.LevelInfo:
		return "\x1b[32m"
	default:
		return "\x1b[36m"
	}
}

func timestampReplacer(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey && a.Value.Kind() == slog.KindTime {
		return slog.String(slog.TimeKey, a.Value.Time().Format("2006-01-02 15:04:05.000"))
	}
	return a
}

// ---------------------------------------------------------------------------
// Convenience logging functions
// ---------------------------------------------------------------------------

// Debug logs at debug level.
func Debug(msg string, args ...any) {
	logAt(context.Background(), slog.LevelDebug, 3, msg, args...)
}

// Info logs at info level.
func Info(msg string, args ...any) {
	logAt(context.Background(), slog.LevelInfo, 3, msg, args...)
}

// Warn logs at warn level.
func Warn(msg string, args ...any) {
	logAt(context.Background(), slog.LevelWarn, 3, msg, args...)
}

// Error logs at error level.
func Error(msg string, args ...any) {
	logAt(context.Background(), slog.LevelError, 3, msg, args...)
}

// Fatal logs at error level and exits.
func Fatal(msg string, args ...any) {
	logAt(context.Background(), slog.LevelError, 3, msg, args...)
	os.Exit(1)
}

// DebugCtx logs at debug level with context.
func DebugCtx(ctx context.Context, msg string, args ...any) {
	logAt(ctx, slog.LevelDebug, 3, msg, args...)
}

// InfoCtx logs at info level with context.
func InfoCtx(ctx context.Context, msg string, args ...any) {
	logAt(ctx, slog.LevelInfo, 3, msg, args...)
}

// WarnCtx logs at warn level with context.
func WarnCtx(ctx context.Context, msg string, args ...any) {
	logAt(ctx, slog.LevelWarn, 3, msg, args...)
}

// ErrorCtx logs at error level with context.
func ErrorCtx(ctx context.Context, msg string, args ...any) {
	logAt(ctx, slog.LevelError, 3, msg, args...)
}

// FatalCtx logs at error level with context and then exits.
func FatalCtx(ctx context.Context, msg string, args ...any) {
	logAt(ctx, slog.LevelError, 3, msg, args...)
	os.Exit(1)
}

// logAt 是内部统一日志写入函数，通过 callerSkip 确保捕获正确的调用者 PC。
func logAt(ctx context.Context, level slog.Level, callerSkip int, msg string, args ...any) {
	var pcs [1]uintptr
	runtime.Callers(callerSkip, pcs[:])
	pc := pcs[0]

	logger := L()
	if !logger.Enabled(ctx, level) {
		return
	}
	r := slog.NewRecord(time.Now(), level, msg, pc)
	r.AddAttrs(argsToAttrs(args)...)
	_ = logger.Handler().Handle(ctx, r)
}

// argsToAttrs converts alternating key-value pairs to slog.Attr slices.
func argsToAttrs(args []any) []slog.Attr {
	if len(args) == 0 {
		return nil
	}
	attrs := make([]slog.Attr, 0, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			continue
		}
		attrs = append(attrs, slog.Any(key, args[i+1]))
	}
	return attrs
}

// ---------------------------------------------------------------------------
// Audit logging
// ---------------------------------------------------------------------------

// AuditLog writes an audit log entry with the given action, dbType, status,
// and optional details.
func AuditLog(action, dbType, status string, details ...string) {
	al := Audit()
	if al == nil {
		return
	}

	attrs := []slog.Attr{
		slog.String("action", action),
		slog.String("db_type", dbType),
		slog.String("status", status),
	}
	if len(details) > 0 {
		attrs = append(attrs, slog.String("details", strings.Join(details, ", ")))
	}

	al.LogAttrs(context.Background(), slog.LevelInfo, "audit", attrs...)
}
