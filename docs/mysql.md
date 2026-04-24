# MySQL 数据库备份与还原命令手册

本文档基于 `backup` 包中的 `MySQLBackup` 实现，汇总了备份、还原、备份管理及恢复操作的底层 **mysql** 和 **mysqldump** 命令。这些命令可直接在数据库服务器上执行，也可通过 Go 程序调用。

---

## 📋 目录

- [MySQL 数据库备份与还原命令手册](#mysql-数据库备份与还原命令手册)
  - [📋 目录](#-目录)
  - [1. 备份命令](#1-备份命令)
    - [1.1 全量备份](#11-全量备份)
    - [1.2 逻辑备份](#12-逻辑备份)
    - [1.3 压缩备份](#13-压缩备份)
  - [2. 还原命令](#2-还原命令)
    - [2.1 还原到原数据库](#21-还原到原数据库)
    - [2.2 还原到新数据库](#22-还原到新数据库)
  - [3. 备份管理命令](#3-备份管理命令)
    - [3.1 列出所有备份](#31-列出所有备份)
    - [3.2 查看备份详细信息](#32-查看备份详细信息)
    - [3.3 删除指定备份](#33-删除指定备份)
    - [3.4 删除所有备份](#34-删除所有备份)
  - [4. Go 代码与底层命令映射](#4-go-代码与底层命令映射)
  - [5. 常用辅助查询](#5-常用辅助查询)
    - [5.1 查看数据库列表](#51-查看数据库列表)
    - [5.2 查看表结构](#52-查看表结构)
    - [5.3 查看当前数据库](#53-查看当前数据库)
  - [6. 典型故障处理](#6-典型故障处理)
    - [6.1 备份时提示权限不足](#61-备份时提示权限不足)
    - [6.2 还原时提示数据库不存在](#62-还原时提示数据库不存在)
    - [6.3 mysqldump 命令未找到](#63-mysqldump-命令未找到)
  - [7. 异机恢复指南](#7-异机恢复指南)
    - [7.1 前提条件](#71-前提条件)
    - [7.2 恢复步骤](#72-恢复步骤)
    - [7.3 命令示例](#73-命令示例)
    - [7.4 注意事项](#74-注意事项)

---

## 1. 备份命令

MySQL 支持通过 `mysqldump` 工具进行逻辑备份，备份文件为 SQL 格式。

### 1.1 全量备份

```bash
mysqldump -h hostname -P port -u username -ppassword database_name > /backup/database_name_20260415_150405.sql
```

### 1.2 逻辑备份

逻辑备份使用 `--single-transaction` 和 `--quick` 选项，适用于 InnoDB 引擎：

```bash
mysqldump -h hostname -P port -u username -ppassword --single-transaction --quick --lock-tables=false database_name > /backup/database_name_20260415_150405.sql
```

### 1.3 压缩备份

```bash
mysqldump -h hostname -P port -u username -ppassword --compress database_name | gzip > /backup/database_name_20260415_150405.sql.gz
```

---

## 2. 还原命令

> **🔄 还原注意事项**
>
> 还原操作会覆盖现有的数据库文件，请在执行还原前确保已备份重要数据。
> 还原过程中数据库将不可用，建议在维护窗口内执行。

### 2.1 还原到原数据库

```bash
mysql -h hostname -P port -u username -ppassword database_name < /backup/database_name_20260415_150405.sql
```

### 2.2 还原到新数据库

```bash
mysql -h hostname -P port -u username -ppassword -e "CREATE DATABASE IF NOT EXISTS new_database_name;"
mysql -h hostname -P port -u username -ppassword new_database_name < /backup/database_name_20260415_150405.sql
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
> 本部分列出了 `MySQLBackup` 实现中的 Go 方法与底层 mysql/mysqldump 命令的对应关系。

| Go 方法                      | 对应的底层命令                                                                 |
| ---------------------------- | ------------------------------------------------------------------------------ |
| `Backup()`                   | `mysqldump -h <host> -P <port> -u <user> -p<password> [--compress] [--single-transaction] <database>` |
| `Restore()`                  | `mysql -h <host> -P <port> -u <user> -p<password> <database> < <backup_file>`  |
| `ListBackups()`              | 遍历文件系统，查找 `*.sql*` 文件                                                |
| `DeleteBackup(backupPath)`   | `rm <backup_path>`                                                              |
| `DeleteAllBackups()`         | `rm *.sql*`                                                                     |
| `ValidateBackup()`           | **不支持**（MySQL 逻辑备份文件无法完全验证有效性）                               |
| `GetBackupInfo(backupPath)`  | 获取文件元信息（大小、修改时间等）                                               |
| `RegisterBackup()`           | **不支持**（MySQL 不使用备份目录库）                                            |
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
SHOW DATABASES;
```

### 5.2 查看表结构

```sql
USE database_name;
DESCRIBE table_name;
```

### 5.3 查看当前数据库

```sql
SELECT DATABASE();
```

---

## 6. 典型故障处理

> **🛠️ 故障处理指南**
>
> 本部分提供了一些常见故障的处理方法，帮助您快速解决备份和恢复过程中遇到的问题。

### 6.1 备份时提示权限不足

```sql
GRANT ALL PRIVILEGES ON database_name.* TO 'username'@'host';
FLUSH PRIVILEGES;
```

### 6.2 还原时提示数据库不存在

```sql
CREATE DATABASE IF NOT EXISTS database_name;
```

### 6.3 mysqldump 命令未找到

确保 MySQL bin 目录已添加到 PATH 环境变量：

```bash
export PATH=/path/to/mysql/bin:$PATH
```

---

## 7. 异机恢复指南

> **📋 异机恢复指南**
>
> 异机恢复是将数据库从一台机器恢复到另一台机器的过程，适用于灾难恢复、系统迁移等场景。

### 7.1 前提条件

- **平台兼容性**：源机器和目标机器可以是不同平台（Windows/Linux）
- **MySQL 版本**：目标数据库的 MySQL 版本应与备份时的版本相同或更高
- **备份文件完整性**：确保备份文件被完整复制
- **MySQL 环境**：目标机器上已正确安装 MySQL 数据库软件

### 7.2 恢复步骤

1. **准备环境**：在目标机器上安装 MySQL 数据库软件
2. **复制备份文件**：将源机器上的备份文件复制到目标机器的相应目录
3. **创建数据库**：在目标机器上创建目标数据库
4. **还原数据库**：使用 mysql 命令还原数据库

### 7.3 命令示例

```bash
mysql -h localhost -u root -ppassword -e "CREATE DATABASE IF NOT EXISTS database_name;"
mysql -h localhost -u root -ppassword database_name < /backup/database_name_20260415_150405.sql
```

### 7.4 注意事项

- **字符集**：确保目标数据库的字符集与源数据库一致
- **MySQL 环境**：确保目标机器上的 MySQL 环境已正确安装
- **备份文件验证**：在恢复前检查备份文件是否完整
- **权限**：确保 MySQL 用户对备份文件和目标目录有足够的权限
