package backup

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRunCapture_Success(t *testing.T) {
	if _, err := exec.LookPath("echo"); err != nil {
		t.Skip("echo not found")
	}
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "echo", "hello")
	output, err := runCapture(ctx, "echo", cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output == "" {
		t.Error("output should not be empty")
	}
}

func TestRunCapture_Failure(t *testing.T) {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "nonexistent_tool_xyz")
	_, err := runCapture(ctx, "nonexistent_tool_xyz", cmd)
	if err == nil {
		t.Fatal("expected error for nonexistent command")
	}
	var ce *CommandError
	if !errors.As(err, &ce) {
		t.Fatalf("expected CommandError, got %T: %v", err, err)
	}
	if ce.Tool != "nonexistent_tool_xyz" {
		t.Errorf("Tool = %q, want %q", ce.Tool, "nonexistent_tool_xyz")
	}
}

func TestRunCapture_WithEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on Windows")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not found")
	}
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "sh", "-c", "echo $MY_TEST_RUNNER_VAR")
	output, err := runCapture(ctx, "sh", cmd, withEnv([]string{"MY_TEST_RUNNER_VAR=hello_env"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output == "" {
		t.Error("output should not be empty")
	}
}

func TestRunToFile_Success(t *testing.T) {
	if _, err := exec.LookPath("echo"); err != nil {
		t.Skip("echo not found")
	}
	ctx := context.Background()
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "out.txt")
	cmd := exec.CommandContext(ctx, "echo", "file_content")
	err := runToFile(ctx, "echo", cmd, outputPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if len(data) == 0 {
		t.Error("output file should not be empty")
	}
}

func TestRunToFile_Failure(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "out.txt")
	cmd := exec.CommandContext(ctx, "nonexistent_tool_xyz")
	err := runToFile(ctx, "nonexistent_tool_xyz", cmd, outputPath)
	if err == nil {
		t.Fatal("expected error for nonexistent command")
	}
	var ce *CommandError
	if !errors.As(err, &ce) {
		t.Fatalf("expected CommandError, got %T: %v", err, err)
	}
}

func TestRunStreaming_Success(t *testing.T) {
	if _, err := exec.LookPath("echo"); err != nil {
		t.Skip("echo not found")
	}
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "echo", "stream_line")
	var lines []string
	output, err := runStreaming(ctx, "echo", cmd, withStreaming(func(line string) {
		lines = append(lines, line)
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output == "" {
		t.Error("output should not be empty")
	}
}

func TestMaskCmdString(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want string
	}{
		{
			name: "password flag",
			cmd:  "mysqldump -u root --password=secret -h localhost db",
			want: "mysqldump -u root --password=*** -h localhost db",
		},
		{
			name: "short password",
			cmd:  "mysql -u root -psecret -h localhost",
			want: "mysql -u root -p*** -h localhost",
		},
		{
			name: "dameng USERID",
			cmd:  "dexp USERID=SYSDBA/pass123@localhost:5236",
			want: "dexp USERID=SYSDBA/***@localhost:5236",
		},
		{
			name: "no password",
			cmd:  "pg_dump -h localhost mydb",
			want: "pg_dump -h localhost mydb",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskCmdString(tt.cmd)
			if got != tt.want {
				t.Errorf("maskCmdString(%q) = %q, want %q", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestBuildCmdString(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), "mysqldump", "-u", "root", "-psecret", "db")
	got := buildCmdString(cmd)
	want := "mysqldump -u root -psecret db"
	if got != want {
		t.Errorf("buildCmdString() = %q, want %q", got, want)
	}
}

func TestTruncateOutput(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short string",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "long string",
			input:  "abcdefghijklmnopqrstuvwxyz",
			maxLen: 5,
			want:   "abcde... (截断，总长度 26)",
		},
		{
			name:   "exact length",
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateOutput(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateOutput() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractStderr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "nil error",
			err:  nil,
			want: "",
		},
		{
			name: "shellexec format",
			err:  errors.New("命令执行失败: exit status 1, stderr: Access denied"),
			want: "Access denied",
		},
		{
			name: "no stderr in message",
			err:  errors.New("some other error"),
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractStderr(tt.err)
			if got != tt.want {
				t.Errorf("extractStderr() = %q, want %q", got, tt.want)
			}
		})
	}
}
