package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileSystemBackupManager_DeleteBackup_WithLogFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建备份文件和关联日志文件（精确后缀匹配，无时间戳）
	backupFile := filepath.Join(tmpDir, "dameng_full_20260703.dmp")
	logFile := backupFile + ".log"
	restoreLogFile := backupFile + ".restore.log"

	if err := os.WriteFile(backupFile, []byte("backup data"), 0o644); err != nil {
		t.Fatalf("创建备份文件失败: %v", err)
	}
	if err := os.WriteFile(logFile, []byte("backup log"), 0o644); err != nil {
		t.Fatalf("创建日志文件失败: %v", err)
	}
	if err := os.WriteFile(restoreLogFile, []byte("restore log"), 0o644); err != nil {
		t.Fatalf("创建还原日志文件失败: %v", err)
	}

	mgr := NewFileSystemBackupManager(tmpDir,
		WithLogicalGlob("*.dmp"),
		WithLogFileSuffixes(".log", ".restore.log"))

	if err := mgr.DeleteBackup(context.TODO(), filepath.Base(backupFile), tmpDir); err != nil {
		t.Fatalf("DeleteBackup 失败: %v", err)
	}

	// 验证备份文件已删除
	if _, err := os.Stat(backupFile); !os.IsNotExist(err) {
		t.Error("备份文件应被删除")
	}
	// 验证日志文件已删除
	if _, err := os.Stat(logFile); !os.IsNotExist(err) {
		t.Error("日志文件 .log 应被删除")
	}
	if _, err := os.Stat(restoreLogFile); !os.IsNotExist(err) {
		t.Error("还原日志文件 .restore.log 应被删除")
	}
}

func TestFileSystemBackupManager_DeleteBackup_WithTimestampedLogFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建备份文件和带时间戳后缀的日志文件（与 dexp/dimp 实际输出一致）
	backupFile := filepath.Join(tmpDir, "dameng_full_20260710_120000.dmp")
	backupLog := backupFile + "_20260710_120000.log"
	restoreLog1 := backupFile + "_20260710_140000.restore.log"
	restoreLog2 := backupFile + "_20260711_090000.restore.log"

	if err := os.WriteFile(backupFile, []byte("backup data"), 0o644); err != nil {
		t.Fatalf("创建备份文件失败: %v", err)
	}
	if err := os.WriteFile(backupLog, []byte("backup log"), 0o644); err != nil {
		t.Fatalf("创建备份日志文件失败: %v", err)
	}
	if err := os.WriteFile(restoreLog1, []byte("restore log 1"), 0o644); err != nil {
		t.Fatalf("创建还原日志文件1失败: %v", err)
	}
	if err := os.WriteFile(restoreLog2, []byte("restore log 2"), 0o644); err != nil {
		t.Fatalf("创建还原日志文件2失败: %v", err)
	}

	mgr := NewFileSystemBackupManager(tmpDir,
		WithLogicalGlob("*.dmp"),
		WithLogFileSuffixes(".log", ".restore.log"))

	if err := mgr.DeleteBackup(context.TODO(), filepath.Base(backupFile), tmpDir); err != nil {
		t.Fatalf("DeleteBackup 失败: %v", err)
	}

	// 验证备份文件已删除
	if _, err := os.Stat(backupFile); !os.IsNotExist(err) {
		t.Error("备份文件应被删除")
	}
	// 验证带时间戳的备份日志已删除
	if _, err := os.Stat(backupLog); !os.IsNotExist(err) {
		t.Error("带时间戳的备份日志文件 .log 应被删除")
	}
	// 验证多次还原产生的日志文件都已删除
	if _, err := os.Stat(restoreLog1); !os.IsNotExist(err) {
		t.Error("带时间戳的还原日志文件1 .restore.log 应被删除")
	}
	if _, err := os.Stat(restoreLog2); !os.IsNotExist(err) {
		t.Error("带时间戳的还原日志文件2 .restore.log 应被删除")
	}
}

func TestFileSystemBackupManager_DeleteBackup_MixedLogFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// 同时存在精确后缀和带时间戳的日志文件
	backupFile := filepath.Join(tmpDir, "dameng_full_20260710.dmp")
	exactLog := backupFile + ".log"
	exactRestoreLog := backupFile + ".restore.log"
	tsLog := backupFile + "_20260710_120000.log"
	tsRestoreLog := backupFile + "_20260710_140000.restore.log"

	for _, f := range []string{backupFile, exactLog, exactRestoreLog, tsLog, tsRestoreLog} {
		if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
			t.Fatalf("创建文件失败 %s: %v", f, err)
		}
	}

	mgr := NewFileSystemBackupManager(tmpDir,
		WithLogicalGlob("*.dmp"),
		WithLogFileSuffixes(".log", ".restore.log"))

	if err := mgr.DeleteBackup(context.TODO(), filepath.Base(backupFile), tmpDir); err != nil {
		t.Fatalf("DeleteBackup 失败: %v", err)
	}

	// 所有文件都应被删除
	for _, f := range []string{backupFile, exactLog, exactRestoreLog, tsLog, tsRestoreLog} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("文件应被删除: %s", f)
		}
	}
}

func TestFileSystemBackupManager_DeleteBackup_NoLogSuffixes(t *testing.T) {
	tmpDir := t.TempDir()

	backupFile := filepath.Join(tmpDir, "test.sql")
	logFile := backupFile + ".log"

	if err := os.WriteFile(backupFile, []byte("sql data"), 0o644); err != nil {
		t.Fatalf("创建备份文件失败: %v", err)
	}
	if err := os.WriteFile(logFile, []byte("log data"), 0o644); err != nil {
		t.Fatalf("创建日志文件失败: %v", err)
	}

	// 不配置 logFileSuffixes，日志文件不应被删除
	mgr := NewFileSystemBackupManager(tmpDir,
		WithLogicalGlob("*.sql"))

	if err := mgr.DeleteBackup(context.TODO(), filepath.Base(backupFile), tmpDir); err != nil {
		t.Fatalf("DeleteBackup 失败: %v", err)
	}

	// 备份文件已删除
	if _, err := os.Stat(backupFile); !os.IsNotExist(err) {
		t.Error("备份文件应被删除")
	}
	// 日志文件仍存在（未配置 logFileSuffixes）
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Error("日志文件不应被删除（未配置 logFileSuffixes）")
	}
}

func TestFileSystemBackupManager_DeleteAllBackups_WithLogFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建多个备份文件和关联日志文件（带时间戳后缀）
	backup1 := filepath.Join(tmpDir, "dameng_full_001.dmp")
	backup2 := filepath.Join(tmpDir, "dameng_full_002.dmp")

	for _, f := range []string{backup1, backup2} {
		if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
			t.Fatalf("创建备份文件失败: %v", err)
		}
		if err := os.WriteFile(f+"_20260710_120000.log", []byte("log"), 0o644); err != nil {
			t.Fatalf("创建日志文件失败: %v", err)
		}
		if err := os.WriteFile(f+"_20260710_140000.restore.log", []byte("rlog"), 0o644); err != nil {
			t.Fatalf("创建还原日志文件失败: %v", err)
		}
	}

	mgr := NewFileSystemBackupManager(tmpDir,
		WithLogicalGlob("*.dmp"),
		WithLogFileSuffixes(".log", ".restore.log"))

	if err := mgr.DeleteAllBackups(context.TODO(), tmpDir); err != nil {
		t.Fatalf("DeleteAllBackups 失败: %v", err)
	}

	// 所有备份文件和日志文件都应被删除
	for _, f := range []string{
		backup1, backup2,
		backup1 + "_20260710_120000.log", backup2 + "_20260710_120000.log",
		backup1 + "_20260710_140000.restore.log", backup2 + "_20260710_140000.restore.log",
	} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("文件应被删除: %s", f)
		}
	}
}

func TestFileSystemBackupManager_DeleteBackup_LogFileNotExist_NoError(t *testing.T) {
	tmpDir := t.TempDir()

	// 只创建备份文件，不创建日志文件
	backupFile := filepath.Join(tmpDir, "dameng_full.dmp")
	if err := os.WriteFile(backupFile, []byte("data"), 0o644); err != nil {
		t.Fatalf("创建备份文件失败: %v", err)
	}

	mgr := NewFileSystemBackupManager(tmpDir,
		WithLogicalGlob("*.dmp"),
		WithLogFileSuffixes(".log", ".restore.log"))

	// 日志文件不存在时不应报错
	if err := mgr.DeleteBackup(context.TODO(), filepath.Base(backupFile), tmpDir); err != nil {
		t.Fatalf("DeleteBackup 不应因缺少日志文件而报错: %v", err)
	}

	if _, err := os.Stat(backupFile); !os.IsNotExist(err) {
		t.Error("备份文件应被删除")
	}
}

func TestFileSystemBackupManager_DeleteBackup_GlobNotMatchingUnrelatedFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建备份文件
	backupFile := filepath.Join(tmpDir, "dameng_full_20260710.dmp")
	if err := os.WriteFile(backupFile, []byte("data"), 0o644); err != nil {
		t.Fatalf("创建备份文件失败: %v", err)
	}

	// 创建另一个备份的日志文件（不应被删除）
	unrelatedBackup := filepath.Join(tmpDir, "dameng_full_20260711.dmp")
	unrelatedLog := unrelatedBackup + "_20260711_120000.log"
	if err := os.WriteFile(unrelatedLog, []byte("unrelated log"), 0o644); err != nil {
		t.Fatalf("创建无关日志文件失败: %v", err)
	}

	mgr := NewFileSystemBackupManager(tmpDir,
		WithLogicalGlob("*.dmp"),
		WithLogFileSuffixes(".log", ".restore.log"))

	if err := mgr.DeleteBackup(context.TODO(), filepath.Base(backupFile), tmpDir); err != nil {
		t.Fatalf("DeleteBackup 失败: %v", err)
	}

	// 备份文件已删除
	if _, err := os.Stat(backupFile); !os.IsNotExist(err) {
		t.Error("备份文件应被删除")
	}

	// 无关日志文件不应被删除
	if _, err := os.Stat(unrelatedLog); os.IsNotExist(err) {
		t.Error("无关备份的日志文件不应被删除")
	}
}
