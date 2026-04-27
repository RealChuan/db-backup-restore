# PostgreSQL 数据库备份与还原命令手册

本文档基于 `backup` 包中的 `PostgreSQLBackup` 实现，汇总了备份、还原、备份管理及恢复操作的底层 **pg_dump**、**pg_dumpall** 和 **psql** 命令。这些命令可直接在数据库服务器上执行，也可通过 Go 程序调用。

---

## 📋 目录

- [PostgreSQL 数据库备份与还原命令手册](#postgresql-数据库备份与还原命令手册)
  - [📋 目录](#-目录)
  - [1. 备份命令](#1-备份命令)
    - [1.1 逻辑备份（SQL 格式）](#11-逻辑备份sql-格式)
    - [1.2 物理备份（目录格式）](#12-物理备份目录格式)
    - [1.3 压缩备份](#13-压缩备份)
    - [1.4 并行备份](#14-并行备份)
    - [1.5 备份所有数据库](#15-备份所有数据库)
  - [2. 还原命令](#2-还原命令)
    - [2.1 还原逻辑备份](#21-还原逻辑备份)
    - [2.2 还原物理备份](#22-还原物理备份)
    - [2.3 还原到新数据库](#23-还原到新数据库)
  - [3. 备份管理命令](#3-备份管理命令)
    - [3.1 列出所有备份](#31-列出所有备份)
    - [3.2 查看备份详细信息](#32-查看备份详细信息)
    - [3.3 删除指定备份](#33-删除指定备份)
    - [3.4 删除所有备份](#34-删除所有备份)
  - [4. Go 代码与底层命令映射](#4-go-代码与底层命令映射)
  - [5. 常用辅助查询](#5-常用辅助查询)
    - [5.1 查看数据库列表](#51-查看数据库列表)
    - [5.2 创建数据库](#52-创建数据库)
    - [5.3 检查数据库是否存在](#53-检查数据库是否存在)
    - [5.4 查看表结构](#54-查看表结构)
  - [6. 典型故障处理](#6-典型故障处理)
    - [6.1 备份时提示权限不足](#61-备份时提示权限不足)
    - [6.2 还原时提示数据库不存在](#62-还原时提示数据库不存在)
    - [6.3 pg\_dump 命令未找到](#63-pg_dump-命令未找到)
    - [6.4 连接被拒绝](#64-连接被拒绝)
  - [7. 异机恢复指南](#7-异机恢复指南)
    - [7.1 前提条件](#71-前提条件)
    - [7.2 恢复步骤](#72-恢复步骤)
    - [7.3 命令示例](#73-命令示例)
    - [7.4 注意事项](#74-注意事项)

---

## 1. 备份命令

PostgreSQL 支持通过 `pg_dump` 工具进行逻辑备份和物理备份（目录格式）。

### 1.1 逻辑备份（SQL 格式）

```bash
pg_dump -h hostname -p port -U username -d database_name --clean --if-exists -f /backup/database_name_20260415_150405.sql
```

### 1.2 物理备份（目录格式）

物理备份使用目录格式，支持并行和压缩：

```bash
pg_dump -h hostname -p port -U username -d database_name -F d -f /backup/database_name_20260415_150405
```

### 1.3 压缩备份

物理备份支持压缩选项：

```bash
pg_dump -h hostname -p port -U username -d database_name -F d -Z 6 -f /backup/database_name_20260415_150405
```

### 1.4 并行备份

物理备份支持并行导出：

```bash
pg_dump -h hostname -p port -U username -d database_name -F d -j 4 -Z 6 -f /backup/database_name_20260415_150405
```

### 1.5 备份所有数据库

使用 `pg_dumpall` 备份所有数据库：

```bash
pg_dumpall -h hostname -p port -U username -f /backup/all_databases_20260415_150405.sql
```

---

## 2. 还原命令

> **🔄 还原注意事项**
>
> 还原操作会覆盖现有的数据库文件，请在执行还原前确保已备份重要数据。
> 还原过程中数据库将不可用，建议在维护窗口内执行。

### 2.1 还原逻辑备份

```bash
psql -h hostname -p port -U username -d database_name -f /backup/database_name_20260415_150405.sql
```

### 2.2 还原物理备份

使用 `pg_restore` 还原目录格式的物理备份：

```bash
pg_restore -h hostname -p port -U username -d database_name -F d /backup/database_name_20260415_150405
```

### 2.3 还原到新数据库

```bash
createdb -h hostname -p port -U username new_database_name
psql -h hostname -p port -U username -d new_database_name -f /backup/database_name_20260415_150405.sql
```

---

## 3. 备份管理命令

> **📊 备份管理指南**
>
> 定期管理备份文件可以确保备份的有效性和可用性，同时避免磁盘空间浪费。

### 3.1 列出所有备份

```bash
ls -la /backup/*.sql*
```

### 3.2 查看备份详细信息

```bash
ls -la /backup/database_name_20260415_150405.sql
```

### 3.3 删除指定备份

```bash
rm /backup/database_name_20260415_150405.sql
```

### 3.4 删除所有备份

```bash
rm /backup/*.sql*
```

---

## 4. Go 代码与底层命令映射

> **🔗 代码映射指南**
>
> 本部分列出了 `PostgreSQLBackup` 实现中的 Go 方法与底层 pg_dump/pg_dumpall/psql 命令的对应关系。

| Go 方法                      | 对应的底层命令                                                                 |
| ---------------------------- | ------------------------------------------------------------------------------ |
| `Backup(BackupLogical)`      | `pg_dump -h <host> -p <port> -U <user> -d <database> -F p --clean --if-exists -f <path>` |
| `Backup(BackupFull)`         | `pg_dump -h <host> -p <port> -U <user> -d <database> -F p --clean --if-exists -f <path>` |
| `Backup(BackupPhysical)`     | `pg_dump -h <host> -p <port> -U <user> -d <database> -F d [-Z <level>] [-j <parallel>] -f <path>` |
| `Restore()`                  | `psql -h <host> -p <port> -U <user> -d <database> -f <backup_file>`            |
| `ListBackups()`              | 遍历文件系统，查找 `*.sql*` 文件                                                |
| `DeleteBackup(backupPath)`   | `rm <backup_path>`                                                              |
| `DeleteAllBackups()`         | `rm *.sql*`                                                                     |
| `ValidateBackup()`           | **不支持**（PostgreSQL 逻辑备份文件无法完全验证有效性）                          |
| `GetBackupInfo(backupPath)`  | 获取文件元信息（大小、修改时间等）                                               |
| `RegisterBackup()`           | **不支持**（PostgreSQL 不使用备份目录库）                                       |
| `UnregisterBackup()`         | **不支持**                                                                     |
| `VerifyBackupStatus()`       | **不支持**                                                                     |
| `DeleteInvalidBackups()`     | **不支持**                                                                     |

---

## 5. 常用辅助查询

> **🔍 辅助查询指南**
>
> 本部分提供了一些常用的 SQL 查询语句，用于监控数据库状态和备份情况。

### 5.1 查看数据库列表

```sql
SELECT datname FROM pg_database WHERE datistemplate = false;
```

### 5.2 创建数据库

```sql
CREATE DATABASE database_name;
```

### 5.3 检查数据库是否存在

```sql
SELECT 1 FROM pg_database WHERE datname = 'database_name';
```

### 5.4 查看表结构

```sql
\d table_name;
```

---

## 6. 典型故障处理

> **🛠️ 故障处理指南**
>
> 本部分提供了一些常见故障的处理方法，帮助您快速解决备份和恢复过程中遇到的问题。

### 6.1 备份时提示权限不足

```sql
GRANT ALL PRIVILEGES ON DATABASE database_name TO username;
```

### 6.2 还原时提示数据库不存在

```sql
CREATE DATABASE database_name;
```

### 6.3 pg_dump 命令未找到

确保 PostgreSQL bin 目录已添加到 PATH 环境变量：

```bash
export PATH=/path/to/postgresql/bin:$PATH
```

### 6.4 连接被拒绝

检查 PostgreSQL 服务是否启动，并确保监听地址正确：

```bash
# 检查服务状态
systemctl status postgresql

# 检查监听配置
cat /var/lib/postgresql/<version>/main/postgresql.conf | grep listen_addresses
```

---

## 7. 异机恢复指南

> **📋 异机恢复指南**
>
> 异机恢复是将数据库从一台机器恢复到另一台机器的过程，适用于灾难恢复、系统迁移等场景。

### 7.1 前提条件

- **平台兼容性**：源机器和目标机器可以是不同平台（Windows/Linux）
- **PostgreSQL 版本**：目标数据库的 PostgreSQL 版本应与备份时的版本相同或更高
- **备份文件完整性**：确保备份文件被完整复制
- **PostgreSQL 环境**：目标机器上已正确安装 PostgreSQL 数据库软件

### 7.2 恢复步骤

1. **准备环境**：在目标机器上安装 PostgreSQL 数据库软件
2. **复制备份文件**：将源机器上的备份文件复制到目标机器的相应目录
3. **创建数据库**：在目标机器上创建目标数据库
4. **还原数据库**：使用 psql 或 pg_restore 命令还原数据库

### 7.3 命令示例

```bash
# 创建数据库
createdb -h localhost -U postgres database_name

# 还原逻辑备份
psql -h localhost -U postgres -d database_name -f /backup/database_name_20260415_150405.sql

# 还原物理备份
pg_restore -h localhost -U postgres -d database_name -F d /backup/database_name_20260415_150405
```

### 7.4 注意事项

- **字符集**：确保目标数据库的字符集与源数据库一致
- **PostgreSQL 环境**：确保目标机器上的 PostgreSQL 环境已正确安装
- **备份文件验证**：在恢复前检查备份文件是否完整
- **权限**：确保 PostgreSQL 用户对备份文件和目标目录有足够的权限
- **pg_hba.conf**：确保目标机器的 `pg_hba.conf` 配置允许连接
