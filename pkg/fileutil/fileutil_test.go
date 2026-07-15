package fileutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestIsWindows(t *testing.T) {
	// 不加 t.Parallel()，因为结果依赖当前运行平台
	result := IsWindows()
	expected := runtime.GOOS == "windows"
	if result != expected {
		t.Errorf("IsWindows() = %v, want %v (runtime.GOOS = %q)", result, expected, runtime.GOOS)
	}
}

func TestAddExeExt(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "空字符串",
			path: "",
			want: "",
		},
		{
			name: "无扩展名的路径",
			path: "mysqldump",
			want: "mysqldump",
		},
		{
			name: "已有扩展名的路径",
			path: "script.sh",
			want: "script.sh",
		},
		{
			name: "路径中含目录且无扩展名",
			path: filepath.Join("bin", "app"),
			want: filepath.Join("bin", "app"),
		},
		{
			name: "路径中含目录且有扩展名",
			path: filepath.Join("bin", "app.exe"),
			want: filepath.Join("bin", "app.exe"),
		},
	}

	// Windows 下无扩展名时应自动加 .exe
	if IsWindows() {
		tests[0].want = ".exe"
		tests[1].want = "mysqldump.exe"
		tests[3].want = filepath.Join("bin", "app.exe")
	}

	// 不加 t.Parallel()，因为结果依赖平台
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AddExeExt(tt.path)
			if got != tt.want {
				t.Errorf("AddExeExt(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestFormatFileSize(t *testing.T) {
	tests := []struct {
		name string
		size int64
		want string
	}{
		{name: "0字节", size: 0, want: "0 bytes"},
		{name: "1字节", size: 1, want: "1 bytes"},
		{name: "1023字节", size: 1023, want: "1023 bytes"},
		{name: "1KB", size: 1024, want: "1.00 KB"},
		{name: "1MB", size: 1024 * 1024, want: "1.00 MB"},
		{name: "1GB", size: 1024 * 1024 * 1024, want: "1.00 GB"},
		{name: "1TB", size: 1024 * 1024 * 1024 * 1024, want: "1.00 TB"},
		{name: "1PB", size: 1024 * 1024 * 1024 * 1024 * 1024, want: "1.00 PB"},
		{name: "1EB", size: 1024 * 1024 * 1024 * 1024 * 1024 * 1024, want: "1.00 EB"},
		{name: "非整KB", size: 1536, want: "1.50 KB"},
		{name: "非整MB", size: 2*1024*1024 + 512*1024, want: "2.50 MB"},
		{name: "大字节数", size: 999, want: "999 bytes"},
		{name: "刚好超过1KB", size: 1025, want: "1.00 KB"},
	}

	t.Parallel()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatFileSize(tt.size)
			if got != tt.want {
				t.Errorf("FormatFileSize(%d) = %q, want %q", tt.size, got, tt.want)
			}
		})
	}
}

func TestEnsureDir(t *testing.T) {
	t.Parallel()

	t.Run("创建新目录", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "newdir")
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("目录 %q 不应已存在", dir)
		}
		if err := EnsureDir(dir); err != nil {
			t.Fatalf("EnsureDir(%q) 返回错误: %v", dir, err)
		}
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("os.Stat(%q) 返回错误: %v", dir, err)
		}
		if !info.IsDir() {
			t.Errorf("EnsureDir 创建的路径不是目录")
		}
	})

	t.Run("目录已存在", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir() // 由测试框架创建，保证已存在
		if err := EnsureDir(dir); err != nil {
			t.Errorf("EnsureDir(%q) 对已存在的目录返回错误: %v", dir, err)
		}
	})

	t.Run("创建嵌套目录", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "a", "b", "c")
		if err := EnsureDir(dir); err != nil {
			t.Fatalf("EnsureDir(%q) 返回错误: %v", dir, err)
		}
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("os.Stat(%q) 返回错误: %v", dir, err)
		}
		if !info.IsDir() {
			t.Errorf("EnsureDir 创建的嵌套路径不是目录")
		}
	})
}

func TestGetDirSize(t *testing.T) {
	t.Parallel()

	t.Run("空目录", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		got := GetDirSize(dir)
		if got != 0 {
			t.Errorf("GetDirSize(空目录) = %d, want 0", got)
		}
	})

	t.Run("包含单个文件的目录", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		content := []byte("hello world")
		if err := os.WriteFile(filepath.Join(dir, "test.txt"), content, 0o644); err != nil {
			t.Fatalf("创建测试文件失败: %v", err)
		}
		got := GetDirSize(dir)
		if got != int64(len(content)) {
			t.Errorf("GetDirSize = %d, want %d", got, len(content))
		}
	})

	t.Run("包含多个文件的目录", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		content1 := []byte("aaa")
		content2 := []byte("bbbbb")
		if err := os.WriteFile(filepath.Join(dir, "f1.txt"), content1, 0o644); err != nil {
			t.Fatalf("创建测试文件失败: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "f2.txt"), content2, 0o644); err != nil {
			t.Fatalf("创建测试文件失败: %v", err)
		}
		want := int64(len(content1) + len(content2))
		got := GetDirSize(dir)
		if got != want {
			t.Errorf("GetDirSize = %d, want %d", got, want)
		}
	})

	t.Run("嵌套目录含文件", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		subDir := filepath.Join(dir, "sub")
		if err := os.MkdirAll(subDir, 0o755); err != nil {
			t.Fatalf("创建子目录失败: %v", err)
		}
		content1 := []byte("top")
		content2 := []byte("nested_file_content")
		if err := os.WriteFile(filepath.Join(dir, "top.txt"), content1, 0o644); err != nil {
			t.Fatalf("创建测试文件失败: %v", err)
		}
		if err := os.WriteFile(filepath.Join(subDir, "nested.txt"), content2, 0o644); err != nil {
			t.Fatalf("创建测试文件失败: %v", err)
		}
		want := int64(len(content1) + len(content2))
		got := GetDirSize(dir)
		if got != want {
			t.Errorf("GetDirSize = %d, want %d", got, want)
		}
	})

	t.Run("不存在的目录", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "nonexistent")
		got := GetDirSize(dir)
		// 不存在的目录应返回 0
		if got != 0 {
			t.Errorf("GetDirSize(不存在的目录) = %d, want 0", got)
		}
	})
}

func TestCopyDir_复制单个文件(t *testing.T) {
	t.Parallel()
	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "dst")

	content := []byte("copy me")
	if err := os.WriteFile(filepath.Join(srcDir, "file.txt"), content, 0o644); err != nil {
		t.Fatalf("创建源文件失败: %v", err)
	}

	if err := CopyDir(srcDir, dstDir); err != nil {
		t.Fatalf("CopyDir 返回错误: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dstDir, "file.txt"))
	if err != nil {
		t.Fatalf("读取目标文件失败: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("复制内容 = %q, want %q", got, content)
	}
}

func TestCopyDir_递归复制嵌套目录(t *testing.T) {
	t.Parallel()
	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "dst")

	subDir := filepath.Join(srcDir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("创建子目录失败: %v", err)
	}

	topContent := []byte("top file")
	nestedContent := []byte("nested file")
	if err := os.WriteFile(filepath.Join(srcDir, "top.txt"), topContent, 0o644); err != nil {
		t.Fatalf("创建源文件失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "nested.txt"), nestedContent, 0o644); err != nil {
		t.Fatalf("创建源文件失败: %v", err)
	}

	if err := CopyDir(srcDir, dstDir); err != nil {
		t.Fatalf("CopyDir 返回错误: %v", err)
	}

	gotTop, err := os.ReadFile(filepath.Join(dstDir, "top.txt"))
	if err != nil {
		t.Fatalf("读取目标文件失败: %v", err)
	}
	if string(gotTop) != string(topContent) {
		t.Errorf("顶层文件内容 = %q, want %q", gotTop, topContent)
	}

	gotNested, err := os.ReadFile(filepath.Join(dstDir, "sub", "nested.txt"))
	if err != nil {
		t.Fatalf("读取嵌套目标文件失败: %v", err)
	}
	if string(gotNested) != string(nestedContent) {
		t.Errorf("嵌套文件内容 = %q, want %q", gotNested, nestedContent)
	}
}

func TestCopyDir_保留目录结构(t *testing.T) {
	t.Parallel()
	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "dst")

	dirs := []string{
		filepath.Join(srcDir, "a"),
		filepath.Join(srcDir, "a", "b"),
		filepath.Join(srcDir, "c"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}
	}

	if err := CopyDir(srcDir, dstDir); err != nil {
		t.Fatalf("CopyDir 返回错误: %v", err)
	}

	for _, relDir := range []string{"a", filepath.Join("a", "b"), "c"} {
		info, err := os.Stat(filepath.Join(dstDir, relDir))
		if err != nil {
			t.Errorf("目录 %q 不存在于目标中: %v", relDir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("路径 %q 不是目录", relDir)
		}
	}
}

func TestCopyDir_源目录不存在(t *testing.T) {
	t.Parallel()
	srcDir := filepath.Join(t.TempDir(), "nonexistent")
	dstDir := filepath.Join(t.TempDir(), "dst")

	err := CopyDir(srcDir, dstDir)
	if err == nil {
		t.Error("CopyDir 对不存在的源目录应返回错误，得到 nil")
	}
}

func TestCopyDir_复制空目录(t *testing.T) {
	t.Parallel()
	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "dst")

	if err := CopyDir(srcDir, dstDir); err != nil {
		t.Fatalf("CopyDir(空目录) 返回错误: %v", err)
	}

	info, err := os.Stat(dstDir)
	if err != nil {
		t.Fatalf("os.Stat(%q) 返回错误: %v", dstDir, err)
	}
	if !info.IsDir() {
		t.Error("目标路径不是目录")
	}
}
