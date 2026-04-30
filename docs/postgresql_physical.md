# PostgreSQL 物理备份与还原命令手册

本文档基于 `backup` 包中的 `PostgreSQLBackup` 物理备份实现，汇总了使用 **pg_basebackup** 进行物理备份、还原及相关操作的命令。

---

## 📋 目录

- [PostgreSQL 物理备份与还原命令手册](#postgresql-物理备份与还原命令手册)
  - [📋 目录](#-目录)
  - [1. 物理备份命令](#1-物理备份命令)
    - [1.1 执行物理备份](#11-执行物理备份)
    - [1.2 并行物理备份](#12-并行物理备份)
  - [2. 物理还原命令](#2-物理还原命令)
    - [2.1 停止 PostgreSQL 服务](#21-停止-postgresql-服务)
    - [2.2 清空目标数据目录](#22-清空目标数据目录)
    - [2.3 复制备份文件](#23-复制备份文件)
    - [2.4 设置文件权限](#24-设置文件权限)
    - [2.5 启动 PostgreSQL 服务](#25-启动-postgresql-服务)
  - [3. Go 代码与底层命令映射](#3-go-代码与底层命令映射)
  - [4. 异机恢复指南](#4-异机恢复指南)
    - [4.1 前提条件](#41-前提条件)
    - [4.2 恢复步骤](#42-恢复步骤)
    - [4.3 命令示例](#43-命令示例)
    - [4.4 注意事项](#44-注意事项)
  - [5. 典型故障处理](#5-典型故障处理)
    - [5.1 pg\_basebackup 命令未找到](#51-pg_basebackup-命令未找到)
    - [5.2 权限不足](#52-权限不足)
    - [5.3 PostgreSQL 服务无法停止/启动](#53-postgresql-服务无法停止启动)

---

## 1. 物理备份命令

> ⚠️ **注意**
>
> 物理备份需要管理员权限执行，且会备份整个 PostgreSQL 实例。

### 1.1 执行物理备份

```bash
pg_basebackup -D /backup/postgresql_20260415_150405 -X stream
```

### 1.2 并行物理备份

```bash
pg_basebackup -D /backup/postgresql_20260415_150405 -X stream -j 4
```

---

## 2. 物理还原命令

> **🔄 还原注意事项**
>
> 物理还原会还原整个 PostgreSQL 实例，需要停止 PostgreSQL 服务，且会清空目标数据目录。

### 2.1 停止 PostgreSQL 服务

**Linux 系统：**

```bash
pg_ctl stop -D /var/lib/postgresql/<version>/main
# 或
systemctl stop postgresql
```

**Windows 系统：**

```bash
net stop postgresql-x64-<version>
```

### 2.2 清空目标数据目录

```bash
rm -rf /var/lib/postgresql/<version>/main/*
```

### 2.3 复制备份文件

```bash
cp -r /backup/postgresql_20260415_150405/* /var/lib/postgresql/<version>/main/
```

### 2.4 设置文件权限

```bash
chown -R postgres:postgres /var/lib/postgresql/<version>/main
find /var/lib/postgresql/<version>/main -type f -exec chmod 600 {} \;
find /var/lib/postgresql/<version>/main -type d -exec chmod 700 {} \;
```

**Windows 系统：**

```bash
icacls "C:\Program Files\PostgreSQL\<version>\data" /grant "postgres:(OI)(CI)F" /T
```

### 2.5 启动 PostgreSQL 服务

**Linux 系统：**

```bash
pg_ctl start -D /var/lib/postgresql/<version>/main
# 或
systemctl start postgresql
```

**Windows 系统：**

```bash
net start postgresql-x64-<version>
```

---

## 3. Go 代码与底层命令映射

> **🔗 代码映射指南**
>
> 本部分列出了 `PostgreSQLBackup` 物理备份实现中的 Go 方法与底层 pg_basebackup/pg_ctl 命令的对应关系。

| Go 方法 | 对应的底层命令 |
| --- | --- |
| `backupPhysical()` | 创建备份目录并调用 execPgBasebackup |
| `execPgBasebackup()` | `pg_basebackup -D <dir> -X stream [-j <parallel>]` |
| `restorePhysical()` | 停止服务 → 清空目录 → 复制文件 → 设置权限 → 启动服务 |
| `stopPostgreSQLService()` | `pg_ctl stop -D <datadir>` |
| `startPostgreSQLService()` | `pg_ctl start -D <datadir>` |
| `setPostgreSQLFilePermissions()` | `chmod 755` 递归设置目录权限 |

---

## 4. 异机恢复指南

> **📋 异机恢复指南**
>
> 异机恢复是将数据库从一台机器恢复到另一台机器的过程，适用于灾难恢复、系统迁移等场景。

### 4.1 前提条件

- **平台兼容性**：源机器和目标机器可以是不同平台（Windows/Linux）
- **PostgreSQL 版本**：目标数据库的 PostgreSQL 版本应与备份时的版本相同或更高
- **备份文件完整性**：确保备份文件被完整复制
- **PostgreSQL 环境**：目标机器上已正确安装 PostgreSQL 数据库软件

### 4.2 恢复步骤

1. **准备环境**：在目标机器上安装 PostgreSQL 数据库软件
2. **复制备份文件**：将源机器上的备份目录复制到目标机器
3. **停止 PostgreSQL 服务**：确保目标机器上的 PostgreSQL 服务已停止
4. **清空数据目录**：清空目标机器上的 PostgreSQL 数据目录
5. **复制数据**：将备份文件复制到数据目录
6. **设置权限**：设置正确的文件权限
7. **启动服务**：启动 PostgreSQL 服务

### 4.3 命令示例

```bash
scp -r user@source:/backup/postgresql_20260415_150405 /backup/

systemctl stop postgresql
rm -rf /var/lib/postgresql/18/main/*

cp -r /backup/postgresql_20260415_150405/* /var/lib/postgresql/18/main/
chown -R postgres:postgres /var/lib/postgresql/18/main

systemctl start postgresql
```

### 4.4 注意事项

- **数据目录配置**：确保目标机器的 PostgreSQL 数据目录配置正确
- **字符集**：确保目标数据库的字符集与源数据库一致
- **备份文件验证**：在恢复前检查备份文件是否完整
- **权限**：确保执行备份的用户对备份目录和数据目录有足够的权限
- **pg_hba.conf**：确保目标机器的 `pg_hba.conf` 配置允许连接

---

## 5. 典型故障处理

### 5.1 pg_basebackup 命令未找到

确保 PostgreSQL bin 目录已添加到 PATH 环境变量：

```bash
export PATH=/path/to/postgresql/bin:$PATH
```

### 5.2 权限不足

以管理员身份运行命令：

```bash
sudo pg_basebackup -D /backup/postgresql_20260415_150405 -X stream
```

### 5.3 PostgreSQL 服务无法停止/启动

检查服务状态并排查问题：

```bash
# 检查服务状态
systemctl status postgresql

# 查看日志
tail -f /var/log/postgresql/postgresql-<version>-main.log
```
