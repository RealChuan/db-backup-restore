# MySQL 逻辑备份与还原命令手册

本文档基于 `backup` 包中的 `MySQLBackup` 逻辑备份实现，汇总了逻辑备份、还原及相关操作的底层 **mysql** 和 **mysqldump** 命令。

---

## 📋 目录

- [MySQL 逻辑备份与还原命令手册](#mysql-逻辑备份与还原命令手册)
  - [📋 目录](#-目录)
  - [1. 逻辑备份命令](#1-逻辑备份命令)
    - [1.1 备份单个数据库](#11-备份单个数据库)
    - [1.2 备份多个数据库](#12-备份多个数据库)
    - [1.3 备份所有数据库](#13-备份所有数据库)
    - [1.4 压缩备份](#14-压缩备份)
  - [2. 逻辑还原命令](#2-逻辑还原命令)
    - [2.1 还原到原数据库](#21-还原到原数据库)
    - [2.2 还原到新数据库](#22-还原到新数据库)
  - [3. Go 代码与底层命令映射](#3-go-代码与底层命令映射)
  - [4. 常用辅助查询](#4-常用辅助查询)
    - [4.1 查看数据库列表](#41-查看数据库列表)
    - [4.2 查看表结构](#42-查看表结构)
    - [4.3 查看当前数据库](#43-查看当前数据库)
  - [5. 典型故障处理](#5-典型故障处理)
    - [5.1 备份时提示权限不足](#51-备份时提示权限不足)
    - [5.2 还原时提示数据库不存在](#52-还原时提示数据库不存在)
    - [5.3 mysqldump 命令未找到](#53-mysqldump-命令未找到)
  - [🔗 官方文档](#-官方文档)

---

## 1. 逻辑备份命令

MySQL 逻辑备份使用 `mysqldump` 工具，备份文件为 SQL 格式，适用于 InnoDB 引擎。

### 1.1 备份单个数据库

```bash
mysqldump -h hostname -P port -u username -ppassword --single-transaction --quick --lock-tables=false database_name > /backup/database_name_20260415_150405.sql
```

### 1.2 备份多个数据库

```bash
mysqldump -h hostname -P port -u username -ppassword --single-transaction --quick --lock-tables=false db1 > /backup/db1_20260415_150405.sql
mysqldump -h hostname -P port -u username -ppassword --single-transaction --quick --lock-tables=false db2 > /backup/db2_20260415_150405.sql
```

### 1.3 备份所有数据库

```bash
for db in $(mysql -h hostname -P port -u username -ppassword -e "SHOW DATABASES" | grep -v -E "^Database$|information_schema|mysql|performance_schema|sys"); do
    mysqldump -h hostname -P port -u username -ppassword --single-transaction --quick --lock-tables=false $db > /backup/${db}_20260415_150405.sql
done
```

### 1.4 压缩备份

```bash
mysqldump -h hostname -P port -u username -ppassword --single-transaction --quick --lock-tables=false --compress database_name | gzip > /backup/database_name_20260415_150405.sql.gz
```

---

## 2. 逻辑还原命令

> **🔄 还原注意事项**
>
> 还原操作会覆盖现有的数据库文件，请在执行还原前确保已备份重要数据。

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

## 3. Go 代码与底层命令映射

> **🔗 代码映射指南**
>
> 本部分列出了 `MySQLBackup` 逻辑备份实现中的 Go 方法与底层 mysql/mysqldump 命令的对应关系。

| Go 方法 | 对应的底层命令 |
| --- | --- |
| `backupSingleDatabaseLogical()` | `mysqldump -h <host> -P <port> -u <user> -p<password> --single-transaction --quick --lock-tables=false <database>` |
| `backupMultipleDatabasesLogical()` | 循环执行 mysqldump 备份多个数据库 |
| `backupAllDatabasesLogical()` | 获取数据库列表后循环执行 mysqldump |
| `restoreLogical()` | `mysql -h <host> -P <port> -u <user> -p<password> <database> < <backup_file>` |
| `getAllDatabases()` | `mysql -h <host> -P <port> -u <user> -p<password> -e "SHOW DATABASES"` |

---

## 4. 常用辅助查询

### 4.1 查看数据库列表

```sql
SHOW DATABASES;
```

### 4.2 查看表结构

```sql
USE database_name;
DESCRIBE table_name;
```

### 4.3 查看当前数据库

```sql
SELECT DATABASE();
```

---

## 5. 典型故障处理

### 5.1 备份时提示权限不足

```sql
GRANT ALL PRIVILEGES ON database_name.* TO 'username'@'host';
FLUSH PRIVILEGES;
```

### 5.2 还原时提示数据库不存在

```sql
CREATE DATABASE IF NOT EXISTS database_name;
```

### 5.3 mysqldump 命令未找到

确保 MySQL bin 目录已添加到 PATH 环境变量：

```bash
export PATH=/path/to/mysql/bin:$PATH
```

---

## 🔗 官方文档

- [MySQL mysqldump 官方文档](https://dev.mysqlserver.cn/doc/refman/8.4/en/using-mysqldump.html)
