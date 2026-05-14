package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"db-backup-restore/pkg/utils"
)

// isAdmin 检查当前进程是否以管理员身份运行
func (m *MySQLBackup) isAdmin() bool {
	return utils.IsAdmin()
}

// backupPhysical 执行 MySQL 物理备份（备份整个数据库实例）
func (m *MySQLBackup) backupPhysical(ctx context.Context, backupDir string, callback ProgressCallback) (*BackupResult, error) {
	if !m.isAdmin() {
		return nil, errors.New("物理备份需要管理员权限，请以管理员身份运行程序")
	}

	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}

	if callback != nil {
		callback(0, "开始物理备份 MySQL 实例...")
	}

	backupPath := filepath.Join(backupDir, fmt.Sprintf("mysql_%s_physical", time.Now().Format("20060102_150405")))

	if err := os.MkdirAll(backupPath, 0755); err != nil {
		return nil, fmt.Errorf("创建备份目录失败: %w", err)
	}

	if err := m.executePhysicalBackup(ctx, backupPath, callback); err != nil {
		return nil, fmt.Errorf("物理备份失败: %w", err)
	}

	if callback != nil {
		callback(100, "物理备份完成")
	}

	result.BackupFile = backupPath
	result.BackupSize = utils.GetDirSize(backupPath)
	result.Duration = time.Since(startTime)
	result.EndTime = time.Now()
	result.Success = true

	return result, nil
}

// executePhysicalBackup 执行物理备份
func (m *MySQLBackup) executePhysicalBackup(ctx context.Context, backupPath string, callback ProgressCallback) error {
	xtrabackupPath, err := m.getXtrabackupPathOrError()
	if err != nil {
		return err
	}

	utils.Infof("检测到 xtrabackup，使用 Percona XtraBackup 进行物理备份")
	return m.execXtrabackup(ctx, xtrabackupPath, backupPath, callback)
}

// getXtrabackupPath 获取 xtrabackup 命令路径
func (m *MySQLBackup) getXtrabackupPath() string {
	if val, ok := m.config.Extra["XTRABACKUP_BIN_PATH"]; ok && val != "" {
		return utils.AddExeExt(filepath.Join(val, "xtrabackup"))
	}

	if path, err := exec.LookPath("xtrabackup"); err == nil {
		return path
	}
	if path, err := exec.LookPath("innobackupex"); err == nil {
		return path
	}

	return ""
}

// getXtrabackupPathOrError 获取 xtrabackup 命令路径，如果未找到则返回错误
func (m *MySQLBackup) getXtrabackupPathOrError() (string, error) {
	xtrabackupPath := m.getXtrabackupPath()
	if xtrabackupPath == "" {
		return "", errors.New("未检测到 xtrabackup，请安装 Percona XtraBackup")
	}
	return xtrabackupPath, nil
}

// execXtrabackup 使用 Percona XtraBackup 执行物理备份
func (m *MySQLBackup) execXtrabackup(ctx context.Context, xtrabackupPath, backupPath string, callback ProgressCallback) error {
	utils.Infof("使用 Percona XtraBackup 进行物理备份")

	args := []string{
		"--backup",
		"--target-dir=" + backupPath,
		fmt.Sprintf("--host=%s", m.config.Host),
		fmt.Sprintf("--port=%d", m.config.Port),
		fmt.Sprintf("--user=%s", m.config.User),
	}

	if m.config.Password != "" {
		args = append(args, fmt.Sprintf("--password=%s", m.config.Password))
	}

	if callback != nil {
		callback(20, "正在执行 xtrabackup 备份...")
	}

	cmdStr := xtrabackupPath + " " + strings.Join(args, " ")
	utils.LogCommandInfo(cmdStr)

	cmd := exec.CommandContext(ctx, xtrabackupPath, args...)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	stderrBytes, _ := io.ReadAll(stderr)
	stderrOutput := string(stderrBytes)

	if err := cmd.Wait(); err != nil {
		utils.LogCommand(cmdStr, stderrOutput, true)
		return fmt.Errorf("xtrabackup 执行失败: %w, stderr: %s", err, stderrOutput)
	}

	if stderrOutput != "" {
		utils.LogCommand(cmdStr, stderrOutput, false)
	}

	if callback != nil {
		callback(80, "xtrabackup 备份完成，准备 prepare...")
	}

	prepareArgs := []string{
		"--prepare",
		"--target-dir=" + backupPath,
	}

	prepareCmdStr := xtrabackupPath + " " + strings.Join(prepareArgs, " ")
	utils.LogCommandInfo(prepareCmdStr)

	prepareCmd := exec.CommandContext(ctx, xtrabackupPath, prepareArgs...)
	prepareOutput, err := utils.ExecCommand(ctx, prepareCmd)

	if err != nil {
		utils.LogCommand(prepareCmdStr, prepareOutput, true)
		return fmt.Errorf("xtrabackup prepare 失败: %w", err)
	}
	utils.LogCommand(prepareCmdStr, prepareOutput, false)

	utils.Infof("XtraBackup 物理备份完成，备份目录: %s", backupPath)
	return nil
}

// restorePhysical 执行 MySQL 物理还原
func (m *MySQLBackup) restorePhysical(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error) {
	if !m.isAdmin() {
		return nil, errors.New("物理还原需要管理员权限，请以管理员身份运行程序")
	}

	startTime := time.Now()
	result := &RestoreResult{}

	if callback != nil {
		callback(0, "开始执行物理还原...")
	}

	xtrabackupPath, err := m.getXtrabackupPathOrError()
	if err != nil {
		return nil, fmt.Errorf("物理还原失败: %w", err)
	}

	var backupDir string
	if opts.BackupIdentifier != "" {
		backupDir = opts.BackupIdentifier
	}

	if backupDir == "" {
		return nil, errors.New("必须通过 --backup-identifier 参数指定备份目录路径")
	}

	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("备份目录不存在: %s", backupDir)
	}

	if opts.TargetDatabaseName != "" {
		utils.Warnf("物理还原将还原整个 MySQL 实例，指定的目标数据库 [%s] 将被忽略", opts.TargetDatabaseName)
	}

	datadir := m.config.Extra["DATA_DIR"]
	if datadir == "" {
		return nil, errors.New("未配置 MySQL 数据目录，请在配置文件中设置 Extra[\"DATA_DIR\"]")
	}

	if err := validateDataDir(datadir, "mysql"); err != nil {
		return nil, fmt.Errorf("DATA_DIR validation failed: %w", err)
	}

	if callback != nil {
		callback(20, "停止 MySQL 服务...")
	}

	if err := m.stopMySQLService(ctx); err != nil {
		return nil, err
	}

	if callback != nil {
		callback(30, "重命名旧数据目录...")
	}

	oldDir := datadir + "_old_" + time.Now().Format("20060102_150405")
	utils.Infof("正在重命名旧数据目录 %s -> %s", datadir, oldDir)
	if err := os.Rename(datadir, oldDir); err != nil {
		return nil, fmt.Errorf("failed to rename old data dir: %w", err)
	}

	if err := os.MkdirAll(datadir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}

	if callback != nil {
		callback(40, "执行 xtrabackup --copy-back...")
	}

	if err := m.execXtrabackupRestore(ctx, xtrabackupPath, backupDir, datadir, callback); err != nil {
		return nil, err
	}

	if callback != nil {
		callback(80, "设置文件权限...")
	}

	if err := m.setMySQLFilePermissions(datadir); err != nil {
		utils.Warnf("设置文件权限失败: %v", err)
	}

	if callback != nil {
		callback(90, "启动 MySQL 服务...")
	}

	if err := m.startMySQLService(ctx); err != nil {
		return nil, err
	}

	if callback != nil {
		callback(100, "物理还原完成")
	}

	result.Duration = time.Since(startTime)
	result.Success = true

	return result, nil
}

// execXtrabackupRestore 使用 Percona XtraBackup 执行物理还原
func (m *MySQLBackup) execXtrabackupRestore(ctx context.Context, xtrabackupPath, backupDir, datadir string, callback ProgressCallback) error {
	utils.Infof("使用 Percona XtraBackup 进行物理还原")

	args := []string{
		"--copy-back",
		"--src-dir=" + backupDir,
		"--datadir=" + datadir,
	}

	cmdStr := xtrabackupPath + " " + strings.Join(args, " ")
	utils.LogCommandInfo(cmdStr)

	cmd := exec.CommandContext(ctx, xtrabackupPath, args...)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	stderrBytes, _ := io.ReadAll(stderr)
	stderrOutput := string(stderrBytes)

	if err := cmd.Wait(); err != nil {
		utils.LogCommand(cmdStr, stderrOutput, true)
		return fmt.Errorf("xtrabackup --copy-back 执行失败: %w, stderr: %s", err, stderrOutput)
	}

	if stderrOutput != "" {
		utils.LogCommand(cmdStr, stderrOutput, false)
	}

	utils.Infof("XtraBackup 物理还原完成，数据目录: %s", datadir)
	return nil
}

// getMySQLServiceName 获取 MySQL 服务名
func (m *MySQLBackup) getMySQLServiceName() string {
	if svc, ok := m.config.Extra["SERVICE_NAME"]; ok && svc != "" {
		return svc
	}

	if runtime.GOOS == "windows" {
		return "MySQL"
	}

	candidates := []string{"mysqld", "mysql", "mariadb", "percona"}
	for _, candidate := range candidates {
		cmd := exec.Command("systemctl", "is-active", "--quiet", candidate)
		if err := cmd.Run(); err == nil {
			return candidate
		}
	}

	return "mysqld"
}

// stopMySQLService 停止 MySQL 服务
func (m *MySQLBackup) stopMySQLService(ctx context.Context) error {
	serviceName := m.getMySQLServiceName()
	utils.Infof("正在停止 MySQL 服务: %s", serviceName)

	if utils.IsWindows() {
		cmd := exec.CommandContext(ctx, "net", "stop", serviceName)
		if err := cmd.Run(); err == nil {
			return nil
		}
	} else {
		cmd := exec.CommandContext(ctx, "systemctl", "stop", serviceName)
		if err := cmd.Run(); err == nil {
			return nil
		}
		cmd = exec.CommandContext(ctx, "service", serviceName, "stop")
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	return fmt.Errorf("无法停止 MySQL 服务 %s", serviceName)
}

// startMySQLService 启动 MySQL 服务
func (m *MySQLBackup) startMySQLService(ctx context.Context) error {
	serviceName := m.getMySQLServiceName()
	utils.Infof("正在启动 MySQL 服务: %s", serviceName)

	if utils.IsWindows() {
		cmd := exec.CommandContext(ctx, "net", "start", serviceName)
		if err := cmd.Run(); err == nil {
			return nil
		}
	} else {
		cmd := exec.CommandContext(ctx, "systemctl", "start", serviceName)
		if err := cmd.Run(); err == nil {
			return nil
		}
		cmd = exec.CommandContext(ctx, "service", serviceName, "start")
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	return fmt.Errorf("无法启动 MySQL 服务 %s", serviceName)
}

// setMySQLFilePermissions 设置 MySQL 文件权限
func (m *MySQLBackup) setMySQLFilePermissions(datadir string) error {
	return filepath.Walk(datadir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chmod(path, 0755)
	})
}
