package logging

import (
	"log/slog"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// parseLevel
// ---------------------------------------------------------------------------

func TestParseLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantLevel slog.Level
		wantErr   bool
	}{
		{"debug级别", "debug", slog.LevelDebug, false},
		{"info级别", "info", slog.LevelInfo, false},
		{"warn级别", "warn", slog.LevelWarn, false},
		{"error级别", "error", slog.LevelError, false},
		{"大写DEBUG", "DEBUG", slog.LevelDebug, false},
		{"大写INFO", "INFO", slog.LevelInfo, false},
		{"大写WARN", "WARN", slog.LevelWarn, false},
		{"大写ERROR", "ERROR", slog.LevelError, false},
		{"混合大小写", "WaRn", slog.LevelWarn, false},
		{"未知级别", "fatal", slog.LevelInfo, true},
		{"空字符串", "", slog.LevelInfo, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotLevel, err := parseLevel(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseLevel(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if gotLevel != tt.wantLevel {
				t.Errorf("parseLevel(%q) = %v, want %v", tt.input, gotLevel, tt.wantLevel)
			}
		})
	}
}

func TestParseLevel_未知级别错误信息(t *testing.T) {
	t.Parallel()

	_, err := parseLevel("fatal")
	if err == nil {
		t.Fatal("期望返回错误，但返回了 nil")
	}
	if !strings.Contains(err.Error(), "fatal") {
		t.Errorf("错误信息应包含输入值 'fatal'，实际: %v", err)
	}
}

// ---------------------------------------------------------------------------
// argsToAttrs
// ---------------------------------------------------------------------------

func TestArgsToAttrs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []any
		want int // 期望的 attr 数量
	}{
		{"空参数", nil, 0},
		{"空切片", []any{}, 0},
		{"单对键值", []any{"key", "value"}, 1},
		{"多对键值", []any{"k1", "v1", "k2", 123, "k3", true}, 3},
		{"奇数个参数_最后一个被忽略", []any{"key1", "val1", "key2"}, 1},
		{"非字符串键被跳过", []any{123, "val", "good", "val2"}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			attrs := argsToAttrs(tt.args)
			if len(attrs) != tt.want {
				t.Errorf("argsToAttrs() 返回 %d 个 attr, 期望 %d", len(attrs), tt.want)
			}
		})
	}
}

func TestArgsToAttrs_空参数返回nil(t *testing.T) {
	t.Parallel()

	result := argsToAttrs(nil)
	if result != nil {
		t.Errorf("argsToAttrs(nil) = %v, 期望 nil", result)
	}
}

func TestArgsToAttrs_键值内容正确(t *testing.T) {
	t.Parallel()

	attrs := argsToAttrs([]any{"name", "test", "count", 42})
	if len(attrs) != 2 {
		t.Fatalf("期望 2 个 attr, 实际 %d", len(attrs))
	}
	if attrs[0].Key != "name" {
		t.Errorf("第一个 attr 的 Key = %q, 期望 %q", attrs[0].Key, "name")
	}
	if attrs[1].Key != "count" {
		t.Errorf("第二个 attr 的 Key = %q, 期望 %q", attrs[1].Key, "count")
	}
}

// ---------------------------------------------------------------------------
// levelString
// ---------------------------------------------------------------------------

func TestLevelString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		level slog.Level
		want  string
	}{
		{"debug级别", slog.LevelDebug, "DEBUG"},
		{"info级别", slog.LevelInfo, "INFO"},
		{"warn级别", slog.LevelWarn, "WARN"},
		{"error级别", slog.LevelError, "ERROR"},
		{"低于debug的级别", slog.LevelDebug - 4, "DEBUG"},
		{"info+1仍为INFO", slog.LevelInfo + 1, "INFO"},
		{"warn+1仍为WARN", slog.LevelWarn + 1, "WARN"},
		{"error+4仍为ERROR", slog.LevelError + 4, "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := levelString(tt.level); got != tt.want {
				t.Errorf("levelString(%v) = %q, 期望 %q", tt.level, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// DefaultConfig
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig() 返回 nil")
	}

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"Level", cfg.Level, "info"},
		{"Output", cfg.Output, "console"},
		{"Format", cfg.Format, "text"},
		{"LogFile", cfg.LogFile, ""},
		{"AuditLogFile", cfg.AuditLogFile, ""},
		{"MaxFileSizeMB", cfg.MaxFileSizeMB, 100},
		{"MaxBackups", cfg.MaxBackups, 10},
		{"EnableColors", cfg.EnableColors, true},
		{"AddCaller", cfg.AddCaller, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Errorf("DefaultConfig().%s = %v, 期望 %v", tt.name, tt.got, tt.want)
			}
		})
	}
}
