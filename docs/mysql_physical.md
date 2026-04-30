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
    - [2.1 停止 MySQL 服务](#21-停止-mysql-服务)
    - [2.2 清空目标数据目录](#22-清空目标数据目录)
    - [2.3 执行还原（copy-back）](#23-执行还原copy-back)
    - [2.4 设置文件权限](#24-设置文件权限)
    - [2.5 启动 MySQL 服务](#25-启动-mysql-服务)
  - [3. Go 代码与底层命令映射](#3-go-代码与底层命令映射)
  - [4. 异机恢复指南](#4-异机恢复指南)
    - [4.1 前提条件](#41-前提条件)
    - [4.2 恢复步骤](#42-恢复步骤)
    - [4.3 命令示例](#43-命令示例)
    - [4.4 注意事项](#44-注意事项)
  - [5. 典型故障处理](#5-典型故障处理)
    - [5.1 xtrabackup 命令未找到](#51-xtrabackup-命令未找到)
    - [5.2 权限不足](#52-权限不足)
    - [5.3 MySQL 服务无法停止/启动](#53-mysql-服务无法停止启动)
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

---

## 2. 物理还原命令

> **🔄 还原注意事项**
>
> 物理还原会还原整个 MySQL 实例，需要停止 MySQL 服务，且会清空目标数据目录。

### 2.1 停止 MySQL 服务

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

### 2.2 清空目标数据目录

```bash
rm -rf /var/lib/mysql/*
mkdir -p /var/lib/mysql
```

### 2.3 执行还原（copy-back）

```bash
xtrabackup --copy-back \
  --src-dir=/backup/mysql_20260415_150405 \
  --datadir=/var/lib/mysql
```

### 2.4 设置文件权限

```bash
chown -R mysql:mysql /var/lib/mysql
find /var/lib/mysql -type f -exec chmod 644 {} \;
find /var/lib/mysql -type d -exec chmod 755 {} \;
```

### 2.5 启动 MySQL 服务

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

| Go 方法 | 对应的底层命令 |
| --- | --- |
| `backupPhysical()` | 创建备份目录并调用 execXtrabackup |
| `execXtrabackup()` | `xtrabackup --backup --target-dir=<dir> --host=<host> --port=<port> --user=<user> [--password=<pwd>]` |
| | `xtrabackup --prepare --target-dir=<dir>` |
| `restorePhysical()` | 停止服务 → 清空目录 → 执行还原 → 设置权限 → 启动服务 |
| `execXtrabackupRestore()` | `xtrabackup --copy-back --src-dir=<backup> --datadir=<datadir>` |
| `stopMySQLService()` | `systemctl stop mysqld` 或 `net stop MySQL` |
| `startMySQLService()` | `systemctl start mysqld` 或 `net start MySQL` |
| `setMySQLFilePermissions()` | `chmod 755` 递归设置目录权限 |

---

## 4. 异机恢复指南

> **📋 异机恢复指南**
>
> 异机恢复是将数据库从一台机器恢复到另一台机器的过程，适用于灾难恢复、系统迁移等场景。

### 4.1 前提条件

- **平台兼容性**：源机器和目标机器可以是不同平台（Windows/Linux）
- **MySQL 版本**：目标数据库的 MySQL 版本应与备份时的版本相同或更高
- **备份文件完整性**：确保备份文件被完整复制
- **Percona XtraBackup**：目标机器上已安装 Percona XtraBackup
- **MySQL 环境**：目标机器上已正确安装 MySQL 数据库软件

### 4.2 恢复步骤

1. **准备环境**：在目标机器上安装 MySQL 和 Percona XtraBackup
2. **复制备份文件**：将源机器上的备份目录复制到目标机器
3. **停止 MySQL 服务**：确保目标机器上的 MySQL 服务已停止
4. **清空数据目录**：清空目标机器上的 MySQL 数据目录
5. **执行还原**：使用 xtrabackup --copy-back 还原数据
6. **设置权限**：设置正确的文件权限
7. **启动服务**：启动 MySQL 服务

### 4.3 命令示例

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

### 4.4 注意事项

- **数据目录配置**：确保目标机器的 MySQL 数据目录配置正确
- **字符集**：确保目标数据库的字符集与源数据库一致
- **备份文件验证**：在恢复前检查备份文件是否完整
- **权限**：确保执行备份的用户对备份目录和数据目录有足够的权限

---

## 5. 典型故障处理

### 5.1 xtrabackup 命令未找到

安装 Percona XtraBackup：

```bash
# CentOS/RHEL
yum install percona-xtrabackup-80

# Ubuntu/Debian
apt-get install percona-xtrabackup-80
```

### 5.2 权限不足

以管理员身份运行命令：

```bash
sudo xtrabackup --backup ...
```

### 5.3 MySQL 服务无法停止/启动

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
