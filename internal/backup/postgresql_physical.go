package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/RealChuan/db-backup-restore/internal/logging"
	"github.com/RealChuan/db-backup-restore/pkg/fileutil"
	"github.com/RealChuan/db-backup-restore/pkg/shellexec"
	"github.com/RealChuan/db-backup-restore/pkg/svcmgmt"
)

// PostgreSQLBackup 的物理备份相关字段和方法

// isAdmin 检查当前进程是否以管理员身份运行
func (p *PostgreSQLBackup) isAdmin() bool {
	return fileutil.IsAdmin()
}

// backupPhysical 执行 PostgreSQL 物理备份（备份整个数据库实例）
func (p *PostgreSQLBackup) backupPhysical(ctx context.Context, backupDir string, callback ProgressCallback) (*BackupResult, error) {
	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}

	if callback != nil {
		callback(0, "开始物理备份 PostgreSQL 实例...")
	}

	backupPath := filepath.Join(backupDir, fmt.Sprintf("postgresql_%s_physical", time.Now().Format("20060102_150405")))

	if err := os.MkdirAll(backupPath, 0o755); err != nil {
		return nil, fmt.Errorf("创建备份目录失败: %w", err)
	}

	if err := p.executePhysicalBackup(ctx, backupPath, callback); err != nil {
		return nil, fmt.Errorf("物理备份失败: %w", err)
	}

	if callback != nil {
		callback(100, "物理备份完成")
	}

	result.BackupFile = backupPath
	result.BackupSize = fileutil.GetDirSize(backupPath)
	result.Duration = time.Since(startTime)
	result.EndTime = time.Now()

	return result, nil
}

// executePhysicalBackup 执行物理备份
func (p *PostgreSQLBackup) executePhysicalBackup(ctx context.Context, backupPath string, callback ProgressCallback) error {
	pgBasebackupPath, err := p.getPgBasebackupPathOrError()
	if err != nil {
		return err
	}

	logging.Info("检测到 pg_basebackup，使用 PostgreSQL 基础备份进行物理备份")
	return p.execPgBasebackup(ctx, pgBasebackupPath, backupPath, callback)
}

// getPgBasebackupPath 获取 pg_basebackup 命令路径
func (p *PostgreSQLBackup) getPgBasebackupPath() string {
	if val := p.config.GetExtraTyped().PGBinPath(); val != "" {
		return fileutil.AddExeExt(filepath.Join(val, "pg_basebackup"))
	}

	if path, err := exec.LookPath("pg_basebackup"); err == nil {
		return path
	}

	return ""
}

// getPgBasebackupPathOrError 获取 pg_basebackup 命令路径，如果未找到则返回错误
func (p *PostgreSQLBackup) getPgBasebackupPathOrError() (string, error) {
	pgBasebackupPath := p.getPgBasebackupPath()
	if pgBasebackupPath == "" {
		return "", errors.New("未检测到 pg_basebackup，请确保 PostgreSQL 已正确安装")
	}
	return pgBasebackupPath, nil
}

// execPgBasebackup 使用 pg_basebackup 执行物理备份
func (p *PostgreSQLBackup) execPgBasebackup(ctx context.Context, pgBasebackupPath, backupPath string, callback ProgressCallback) error {
	logging.Info("使用 pg_basebackup 进行物理备份")

	args := []string{
		"-D", backupPath,
		"-X", "stream",
	}

	if callback != nil {
		callback(20, "正在执行 pg_basebackup 备份...")
	}

	cmdStr := pgBasebackupPath + " " + strings.Join(args, " ")
	logging.LogCommandInfo(cmdStr)

	cmd := exec.CommandContext(ctx, pgBasebackupPath, args...)
	cmd.Env = append(os.Environ(), p.env...)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	stderrBytes, err := io.ReadAll(stderr)
	if err != nil {
		return fmt.Errorf("读取命令错误输出失败: %w", err)
	}
	stderrOutput := string(stderrBytes)

	if err := cmd.Wait(); err != nil {
		logging.LogCommand(cmdStr, stderrOutput, true)
		return fmt.Errorf("pg_basebackup 执行失败: %w, stderr: %s", err, stderrOutput)
	}

	if stderrOutput != "" {
		logging.LogCommand(cmdStr, stderrOutput, false)
	}

	logging.InfoCtx(ctx, "pg_basebackup 物理备份完成", "backup_dir", backupPath)
	return nil
}

// restorePhysical 执行 PostgreSQL 物理还原（还原整个数据库实例）
//
// 安全设计原则：
//  1. 先还原到临时目录，验证完整性后再切换
//  2. 保留旧数据目录作为备份，不自动删除
//  3. 任何步骤失败都进行回滚，保证数据安全
//
// 执行流程（原子操作保证数据安全）：
//  1. 权限检查 → 2. 参数校验 → 3. 创建临时目录 → 4. 复制备份到临时目录
//  5. 验证临时目录 → 6. 停止服务 → 7. 重命名旧目录 → 8. 切换新目录
//  9. 设置权限 → 10. 启动服务 → 11. 输出清理提示
//
// 关键设计说明：
//   - 临时目录策略：先将备份复制到 {datadir}_new_{timestamp}，验证通过后再切换
//   - 旧目录保留：原数据目录重命名为 {datadir}_old_{timestamp}，由用户手动清理
//   - 回滚机制：任何步骤失败都清理临时目录，必要时恢复原数据目录
func (p *PostgreSQLBackup) restorePhysical(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error) {
	if !p.isAdmin() {
		return nil, errors.New("物理还原需要管理员权限，请以管理员身份运行程序")
	}

	startTime := time.Now()
	result := &RestoreResult{}

	if callback != nil {
		callback(0, "开始执行物理还原...")
	}

	backupDir, datadir, err := p.validateRestorePhysicalParams(ctx, opts)
	if err != nil {
		return nil, err
	}

	timestamp := time.Now().Format("20060102_150405")
	tempDir := datadir + "_new_" + timestamp
	oldDir := datadir + "_old_" + timestamp

	if err := p.restorePhysicalPrepare(ctx, backupDir, tempDir, callback); err != nil {
		return nil, err
	}

	if err := p.stopPostgreSQLService(ctx); err != nil {
		os.RemoveAll(tempDir)
		return nil, err
	}

	if err := p.restorePhysicalSwap(ctx, datadir, tempDir, oldDir, callback); err != nil {
		return nil, err
	}

	if err := p.setPostgreSQLFilePermissions(ctx, datadir); err != nil {
		logging.WarnCtx(ctx, "设置文件权限失败", "error", err)
	}

	if callback != nil {
		callback(90, "准备启动 PostgreSQL 服务...")
	}

	if err := p.startPostgreSQLService(ctx); err != nil {
		return nil, err
	}

	if err := p.waitForPostgreSQL(ctx, datadir); err != nil {
		return nil, fmt.Errorf("PostgreSQL 启动后等待就绪失败: %w", err)
	}

	if callback != nil {
		callback(95, "验证备份...")
	}

	if err := p.validatePhysicalBackup(ctx, datadir); err != nil {
		logging.WarnCtx(ctx, "备份验证失败", "error", err)
	}

	if callback != nil {
		callback(100, "物理还原完成")
	}

	logging.WarnCtx(ctx, "旧数据目录已保留，请手动清理", "dir", oldDir)

	result.Duration = time.Since(startTime)
	return result, nil
}

// validateRestorePhysicalParams 校验物理还原参数
func (p *PostgreSQLBackup) validateRestorePhysicalParams(ctx context.Context, opts RestoreOptions) (backupDir string, datadir string, err error) {
	backupDir = opts.BackupIdentifier
	if backupDir == "" {
		return "", "", errors.New("必须通过 --backup-identifier 参数指定备份目录路径")
	}

	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		return "", "", fmt.Errorf("备份目录不存在: %s", backupDir)
	}

	if opts.TargetDatabaseName != "" {
		logging.WarnCtx(ctx, "物理还原将还原整个实例，指定目标数据库将被忽略", "target_database", opts.TargetDatabaseName)
	}

	datadir = p.config.GetExtraTyped().DataDir()
	if datadir == "" {
		return "", "", errors.New("未配置 PostgreSQL 数据目录，请在配置文件中设置 Extra[\"DATA_DIR\"]")
	}

	if err := validateDataDir(datadir, "postgresql"); err != nil {
		return "", "", fmt.Errorf("DATA_DIR validation failed: %w", err)
	}

	return backupDir, datadir, nil
}

// restorePhysicalPrepare 准备临时目录并复制备份文件
func (p *PostgreSQLBackup) restorePhysicalPrepare(_ context.Context, backupDir, tempDir string, callback ProgressCallback) error {
	if callback != nil {
		callback(20, "创建临时目录...")
	}

	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}

	if callback != nil {
		callback(30, "复制备份文件到临时目录...")
	}

	if err := fileutil.CopyDir(backupDir, tempDir); err != nil {
		os.RemoveAll(tempDir)
		return fmt.Errorf("复制备份文件失败: %w", err)
	}

	if callback != nil {
		callback(50, "验证临时目录...")
	}

	if err := validateDataDir(tempDir, "postgresql"); err != nil {
		os.RemoveAll(tempDir)
		return fmt.Errorf("临时目录验证失败: %w", err)
	}

	return nil
}

// restorePhysicalSwap 停止服务后交换数据目录
func (p *PostgreSQLBackup) restorePhysicalSwap(ctx context.Context, datadir, tempDir, oldDir string, callback ProgressCallback) error {
	if callback != nil {
		callback(60, "停止 PostgreSQL 服务...")
	}

	if err := p.stopPostgreSQLService(ctx); err != nil {
		os.RemoveAll(tempDir)
		return err
	}

	if callback != nil {
		callback(70, "重命名旧数据目录...")
	}

	logging.InfoCtx(ctx, "正在重命名旧数据目录", "from", datadir, "to", oldDir)
	if err := os.Rename(datadir, oldDir); err != nil {
		p.startPostgreSQLService(ctx) //nolint:errcheck
		os.RemoveAll(tempDir)
		return fmt.Errorf("failed to rename old data dir: %w", err)
	}

	if callback != nil {
		callback(80, "切换到新数据目录...")
	}

	logging.InfoCtx(ctx, "正在重命名临时目录", "from", tempDir, "to", datadir)
	if err := os.Rename(tempDir, datadir); err != nil {
		os.Rename(oldDir, datadir)    //nolint:errcheck
		p.startPostgreSQLService(ctx) //nolint:errcheck
		return fmt.Errorf("failed to rename temp dir to datadir: %w", err)
	}

	if callback != nil {
		callback(85, "设置文件权限...")
	}

	return nil
}

// stopPostgreSQLService 停止 PostgreSQL 服务
func (p *PostgreSQLBackup) stopPostgreSQLService(ctx context.Context) error {
	datadir := p.config.GetExtraTyped().DataDir()
	if datadir == "" {
		return errors.New("未配置 PostgreSQL 数据目录，请在配置文件中设置 Extra[\"DATA_DIR\"]")
	}

	args := []string{"stop", "-D", datadir}
	cmdStr := p.pgCtlPath + " " + strings.Join(args, " ")
	logging.LogCommandInfo(cmdStr)

	cmd := exec.CommandContext(ctx, p.pgCtlPath, args...)
	cmd.Env = append(os.Environ(), p.env...)

	output, err := shellexec.ExecCommand(cmd)
	if err != nil {
		logging.LogCommand(cmdStr, output, true)
		return fmt.Errorf("pg_ctl stop 执行失败: %w, output: %s", err, output)
	}
	logging.LogCommand(cmdStr, output, false)
	return nil
}

// startPostgreSQLService 启动 PostgreSQL 服务
func (p *PostgreSQLBackup) startPostgreSQLService(ctx context.Context) error {
	datadir := p.config.GetExtraTyped().DataDir()
	if datadir == "" {
		return errors.New("未配置 PostgreSQL 数据目录，请在配置文件中设置 Extra[\"DATA_DIR\"]")
	}

	if fileutil.IsWindows() {
		return p.startPostgreSQLServiceWindows(ctx)
	}

	args := []string{"start", "-D", datadir}
	cmdStr := p.pgCtlPath + " " + strings.Join(args, " ")
	logging.LogCommandInfo(cmdStr)

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, p.pgCtlPath, args...)
	cmd.Env = append(os.Environ(), p.env...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("pg_ctl start 执行失败: %w, stdout: %s, stderr: %s", err, stdout.String(), stderr.String())
	}

	if err := p.waitForPostgreSQL(ctx, datadir); err != nil {
		return fmt.Errorf("等待 PostgreSQL 启动失败: %w", err)
	}

	return nil
}

// startPostgreSQLServiceWindows 在 Windows 上使用服务方式启动 PostgreSQL
func (p *PostgreSQLBackup) startPostgreSQLServiceWindows(ctx context.Context) error {
	serviceName := p.config.GetExtraTyped().ServiceName()
	if serviceName == "" {
		serviceName = "postgresql-x64-18"
	}

	if err := svcmgmt.StartWindowsService(ctx, serviceName); err != nil {
		return fmt.Errorf("无法启动 PostgreSQL 服务 [%s]: %w", serviceName, err)
	}
	return nil
}

// waitForPostgreSQL 等待 PostgreSQL 启动完成
func (p *PostgreSQLBackup) waitForPostgreSQL(ctx context.Context, datadir string) error {
	maxWait := 30
	for i := 0; i < maxWait; i++ {
		args := []string{"status", "-D", datadir}
		cmd := exec.CommandContext(ctx, p.pgCtlPath, args...)
		if err := cmd.Run(); err == nil {
			logging.Info("PostgreSQL 服务已启动")
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return errors.New("PostgreSQL 启动超时")
}

// setPostgreSQLFilePermissions 设置 PostgreSQL 文件权限
func (p *PostgreSQLBackup) setPostgreSQLFilePermissions(ctx context.Context, datadir string) error {
	if fileutil.IsWindows() {
		return p.setPostgreSQLFilePermissionsWindows(ctx, datadir)
	}
	return filepath.Walk(datadir, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chmod(path, 0o755)
	})
}

// setPostgreSQLFilePermissionsWindows 在 Windows 上设置 PostgreSQL 文件权限
func (p *PostgreSQLBackup) setPostgreSQLFilePermissionsWindows(ctx context.Context, datadir string) error {
	args := []string{"/c", "icacls", datadir, "/grant", "Everyone:(OI)(CI)F", "/T"}
	cmdStr := "cmd " + strings.Join(args, " ")
	logging.LogCommandInfo(cmdStr)

	cmd := exec.CommandContext(ctx, "cmd", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logging.LogCommand(cmdStr, string(output), true)
		return fmt.Errorf("设置文件权限失败: %w, output: %s", err, string(output))
	}
	logging.LogCommand(cmdStr, string(output), false)
	return nil
}

// getPgVerifyBackupPathOrError 获取 pg_verifybackup 命令路径，如果未找到则返回错误
func (p *PostgreSQLBackup) getPgVerifyBackupPathOrError() (string, error) {
	if p.pgVerifyBackupPath == "" {
		return "", errors.New("未检测到 pg_verifybackup，请确保 PostgreSQL 16+ 已正确安装")
	}
	return p.pgVerifyBackupPath, nil
}

// validatePhysicalBackup 使用 pg_verifybackup 验证物理备份的完整性
func (p *PostgreSQLBackup) validatePhysicalBackup(ctx context.Context, backupID string, opts ...BackupOptions) error {
	var backupPath string
	if filepath.IsAbs(backupID) {
		backupPath = backupID
	} else {
		if len(opts) > 0 && opts[0].TargetPath != "" {
			backupPath = filepath.Join(opts[0].TargetPath, backupID)
		} else {
			return errors.New("必须通过 opts.TargetPath 指定备份目录或提供绝对路径")
		}
	}

	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("备份目录不存在: %s", backupPath)
	}

	pgVerifyBackupPath, err := p.getPgVerifyBackupPathOrError()
	if err != nil {
		return err
	}

	logging.InfoCtx(ctx, "使用 pg_verifybackup 验证备份", "backup_dir", backupPath)

	args := []string{backupPath}
	cmdStr := pgVerifyBackupPath + " " + strings.Join(args, " ")
	logging.LogCommandInfo(cmdStr)

	cmd := exec.CommandContext(ctx, pgVerifyBackupPath, args...)
	cmd.Env = append(os.Environ(), p.env...)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	stderrBytes, err := io.ReadAll(stderr)
	if err != nil {
		return fmt.Errorf("读取命令错误输出失败: %w", err)
	}
	stderrOutput := string(stderrBytes)

	if err := cmd.Wait(); err != nil {
		logging.LogCommand(cmdStr, stderrOutput, true)
		return fmt.Errorf("pg_verifybackup 执行失败: %w, stderr: %s", err, stderrOutput)
	}

	if stderrOutput != "" {
		logging.LogCommand(cmdStr, stderrOutput, false)
	}

	logging.InfoCtx(ctx, "pg_verifybackup 验证完成", "backup_dir", backupPath)
	return nil
}
