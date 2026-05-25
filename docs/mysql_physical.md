# MySQL 物理备份与还原命令手册

本文档基于 `backup` 包中的 `MySQLBackup` 物理备份实现，汇总了使用 **Percona XtraBackup** 进行物理备份、还原及相关操作的命令。

---

## 📋 目录

- [MySQL 物理备份与还原命令手册](#mysql-物理备份与还原命令手册)
  - [📋 目录](#-目录)
  - [1. 物理备份命令](#1-物理备份命令)
    - [1.1 执行物理备份](#11-执行物理备份)
    - [1.2 准备备份（prepare）](#12-准备备份prepare)
  - [2. 物理还原命令](#2-物理还原命令)
    - [2.1 创建临时目录](#21-创建临时目录)
    - [2.2 还原到临时目录（copy-back）](#22-还原到临时目录copy-back)
    - [2.3 验证临时目录](#23-验证临时目录)
    - [2.4 停止 MySQL 服务](#24-停止-mysql-服务)
    - [2.5 重命名旧数据目录并切换](#25-重命名旧数据目录并切换)
    - [2.6 设置文件权限](#26-设置文件权限)
    - [2.7 启动 MySQL 服务](#27-启动-mysql-服务)
  - [3. Go 代码与底层命令映射](#3-go-代码与底层命令映射)
    - [备份方法](#备份方法)
    - [还原方法](#还原方法)
    - [服务管理方法](#服务管理方法)
    - [辅助方法](#辅助方法)
  - [4. 配置项](#4-配置项)
  - [5. 异机恢复指南](#5-异机恢复指南)
    - [5.1 前提条件](#51-前提条件)
    - [5.2 恢复步骤](#52-恢复步骤)
    - [5.3 命令示例](#53-命令示例)
    - [5.4 注意事项](#54-注意事项)
  - [6. 典型故障处理](#6-典型故障处理)
    - [6.1 xtrabackup 命令未找到](#61-xtrabackup-命令未找到)
    - [6.2 权限不足](#62-权限不足)
    - [6.3 MySQL 服务无法停止/启动](#63-mysql-服务无法停止启动)
  - [🔗 官方文档](#-官方文档)

---

## 1. 物理备份命令

> ⚠️ **注意**
>
> 物理备份需要管理员权限执行。

### 1.1 执行物理备份

```bash
xtrabackup --backup \
  --target-dir=/backup/mysql_20260415_150405 \
  --host=hostname \
  --port=port \
  --user=username \
  --password=password
```

### 1.2 准备备份（prepare）

```bash
xtrabackup --prepare \
  --target-dir=/backup/mysql_20260415_150405
```

> **注意**：代码中 `execXtrabackup()` 方法会自动依次执行 backup 和 prepare 两步操作。

---

## 2. 物理还原命令

> **🔄 还原注意事项**
>
> 物理还原会还原整个 MySQL 实例，需要停止 MySQL 服务。
> 原数据目录会被重命名为 `{datadir}_old_{timestamp}` 保留，不会自动删除。
>
> **关键设计**：代码采用"临时目录策略"最小化停机时间——先将备份还原到临时目录并验证，确认无误后才停止服务并切换目录。

代码中 `restorePhysical()` 的完整执行流程：

1. **权限检查** → 2. **获取 xtrabackup 路径** → 3. **参数校验** → 4. **创建临时目录** → 5. **还原到临时目录** → 6. **验证临时目录** → 7. **停止服务** → 8. **重命名旧目录** → 9. **切换新目录** → 10. **设置权限** → 11. **启动服务** → 12. **输出清理提示**

### 2.1 创建临时目录

```bash
mkdir -p /var/lib/mysql_new_20260415_150405
```

### 2.2 还原到临时目录（copy-back）

```bash
xtrabackup --copy-back \
  --src-dir=/backup/mysql_20260415_150405 \
  --datadir=/var/lib/mysql_new_20260415_150405
```

### 2.3 验证临时目录

验证数据目录包含 MySQL 特征文件（`ibdata1` 或 `mysql/` 目录），确保还原数据有效。

### 2.4 停止 MySQL 服务

> **注意**：停止服务在还原到临时目录并验证之后执行，以最小化停机时间。

**Linux 系统：**

```bash
systemctl stop mysqld
# 或
service mysql stop
```

**Windows 系统：**

```bash
net stop MySQL
```

### 2.5 重命名旧数据目录并切换

```bash
# 重命名旧数据目录（保留备份）
mv /var/lib/mysql /var/lib/mysql_old_20260415_150405

# 切换到新数据目录
mv /var/lib/mysql_new_20260415_150405 /var/lib/mysql
```

> **回滚机制**：如果切换新目录失败，代码会自动将旧目录恢复原位并重启服务。

### 2.6 设置文件权限

```bash
# 代码使用 chmod 755 递归设置所有文件和目录权限
chmod -R 755 /var/lib/mysql
```

> **注意**：代码对所有文件和目录统一设置 `0755` 权限，而非分别设置 `644`（文件）和 `755`（目录）。

### 2.7 启动 MySQL 服务

**Linux 系统：**

```bash
systemctl start mysqld
# 或
service mysql start
```

**Windows 系统：**

```bash
net start MySQL
```

---

## 3. Go 代码与底层命令映射

> **🔗 代码映射指南**
>
> 本部分列出了 `MySQLBackup` 物理备份实现中的 Go 方法与底层 xtrabackup 命令的对应关系。

### 备份方法

| Go 方法 | 签名 | 说明 |
| --- | --- | --- |
| `backupPhysical()` | `(ctx context.Context, backupDir string, callback ProgressCallback) (*BackupResult, error)` | 权限检查 → 创建备份目录 → 调用 `executePhysicalBackup()` |
| `executePhysicalBackup()` | `(ctx context.Context, backupPath string, callback ProgressCallback) error` | 获取 xtrabackup 路径 → 调用 `execXtrabackup()` |
| `execXtrabackup()` | `(ctx context.Context, xtrabackupPath, backupPath string, callback ProgressCallback) error` | 依次执行：`xtrabackup --backup --target-dir=<dir> --host=<host> --port=<port> --user=<user> [--password=<pwd>]` 和 `xtrabackup --prepare --target-dir=<dir>` |

### 还原方法

| Go 方法 | 签名 | 说明 |
| --- | --- | --- |
| `restorePhysical()` | `(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error)` | 权限检查 → 获取 xtrabackup 路径 → 参数校验 → 创建临时目录 → 还原到临时目录 → 验证 → 停止服务 → 重命名旧目录 → 切换新目录 → 设置权限 → 启动服务 → 输出清理提示 |
| `execXtrabackupRestore()` | `(ctx context.Context, xtrabackupPath, backupDir, datadir string, _ ProgressCallback) error` | `xtrabackup --copy-back --src-dir=<backup> --datadir=<tempdir>` |

### 服务管理方法

| Go 方法 | 签名 | 说明 |
| --- | --- | --- |
| `stopMySQLService()` | `(ctx context.Context) error` | Windows: `net stop <service>`；Linux: `systemctl stop <service>` 或 `service <service> stop` |
| `startMySQLService()` | `(ctx context.Context) error` | Windows: `net start <service>`；Linux: `systemctl start <service>` 或 `service <service> start` |
| `getMySQLServiceName()` | `(ctx context.Context) string` | 优先使用 `SERVICE_NAME` 配置，否则自动检测（Linux 检测 mysqld/mysql/mariadb/percona） |

### 辅助方法

| Go 方法 | 签名 | 说明 |
| --- | --- | --- |
| `isAdmin()` | `() bool` | 检查当前进程是否以管理员身份运行 |
| `getXtrabackupPath()` | `() string` | 获取 xtrabackup 命令路径（优先 `XTRABACKUP_BIN_PATH` 配置，其次查找 PATH 中的 xtrabackup/innobackupex） |
| `getXtrabackupPathOrError()` | `() (string, error)` | 获取 xtrabackup 路径，未找到则返回错误 |
| `setMySQLFilePermissions()` | `(datadir string) error` | 递归设置数据目录权限为 `0755` |
| `validateDataDir()` | `(datadir string, dbType string) error` | 验证数据目录合法性：绝对路径、非根目录、非系统目录、包含 MySQL 特征文件（ibdata1 或 mysql/ 目录） |

---

## 4. 配置项

通过 `DBConfig.Extra` 传入的配置项：

| 配置键 | 说明 | 必填 | 默认值 |
| --- | --- | --- | --- |
| `DATA_DIR` | MySQL 数据目录 | 物理还原时必填 | 无 |
| `XTRABACKUP_BIN_PATH` | XtraBackup 二进制文件目录 | 否 | 系统 PATH 中的 `xtrabackup` 或 `innobackupex` |
| `SERVICE_NAME` | MySQL 服务名 | 否 | Windows: `MySQL`；Linux: 自动检测（mysqld/mysql/mariadb/percona） |

---

## 5. 异机恢复指南

> **📋 异机恢复指南**
>
> 异机恢复是将数据库从一台机器恢复到另一台机器的过程，适用于灾难恢复、系统迁移等场景。

### 5.1 前提条件

- **平台兼容性**：源机器和目标机器可以是不同平台（Windows/Linux）
- **MySQL 版本**：目标数据库的 MySQL 版本应与备份时的版本相同或更高
- **备份文件完整性**：确保备份文件被完整复制
- **Percona XtraBackup**：目标机器上已安装 Percona XtraBackup
- **MySQL 环境**：目标机器上已正确安装 MySQL 数据库软件

### 5.2 恢复步骤

1. **准备环境**：在目标机器上安装 MySQL 和 Percona XtraBackup
2. **复制备份文件**：将源机器上的备份目录复制到目标机器
3. **停止 MySQL 服务**：确保目标机器上的 MySQL 服务已停止
4. **清空数据目录**：清空目标机器上的 MySQL 数据目录
5. **执行还原**：使用 xtrabackup --copy-back 还原数据
6. **设置权限**：设置正确的文件权限
7. **启动服务**：启动 MySQL 服务

### 5.3 命令示例

```bash
scp -r user@source:/backup/mysql_20260415_150405 /backup/

systemctl stop mysqld
rm -rf /var/lib/mysql/*

xtrabackup --copy-back \
  --src-dir=/backup/mysql_20260415_150405 \
  --datadir=/var/lib/mysql

chown -R mysql:mysql /var/lib/mysql
systemctl start mysqld
```

### 5.4 注意事项

- **数据目录配置**：确保目标机器的 MySQL 数据目录配置正确
- **字符集**：确保目标数据库的字符集与源数据库一致
- **备份文件验证**：在恢复前检查备份文件是否完整
- **权限**：确保执行备份的用户对备份目录和数据目录有足够的权限

---

## 6. 典型故障处理

### 6.1 xtrabackup 命令未找到

安装 Percona XtraBackup，或通过 `XTRABACKUP_BIN_PATH` 配置项指定路径：

```bash
# CentOS/RHEL
yum install percona-xtrabackup-80

# Ubuntu/Debian
apt-get install percona-xtrabackup-80
```

### 6.2 权限不足

以管理员身份运行命令：

```bash
sudo xtrabackup --backup ...
```

### 6.3 MySQL 服务无法停止/启动

检查服务状态并排查问题：

```bash
# 检查服务状态
systemctl status mysqld

# 查看日志
tail -f /var/log/mysqld.log
```

---

## 🔗 官方文档

- [Percona XtraBackup 官方文档](https://docs.percona.com/percona-xtrabackup/8.4/)
