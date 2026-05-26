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
  - [2. 逻辑还原命令](#2-逻辑还原命令)
    - [2.1 还原到指定数据库](#21-还原到指定数据库)
  - [3. Go 代码与底层命令映射](#3-go-代码与底层命令映射)
    - [备份方法](#备份方法)
    - [还原方法](#还原方法)
    - [辅助方法](#辅助方法)
  - [4. 配置项](#4-配置项)
  - [5. 常用辅助查询](#5-常用辅助查询)
    - [5.1 查看数据库列表](#51-查看数据库列表)
    - [5.2 查看表结构](#52-查看表结构)
    - [5.3 查看当前数据库](#53-查看当前数据库)
  - [6. 典型故障处理](#6-典型故障处理)
    - [6.1 备份时提示权限不足](#61-备份时提示权限不足)
    - [6.2 还原时提示数据库不存在](#62-还原时提示数据库不存在)
    - [6.3 mysqldump 命令未找到](#63-mysqldump-命令未找到)
  - [🔗 官方文档](#-官方文档)

---

## 1. 逻辑备份命令

MySQL 逻辑备份使用 `mysqldump` 工具，备份文件为 SQL 格式，适用于 InnoDB 引擎。

代码通过 `buildDumpCommandArgs()` 构建的标准参数为：`--single-transaction --quick --lock-tables=false`

> **注意**：当前代码不支持压缩备份（`--compress` / gzip），`BackupOptions.EnableCompression` 仅对 Oracle/PostgreSQL 生效。

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

---

## 2. 逻辑还原命令

> **🔄 还原注意事项**
>
> 还原操作会覆盖现有的数据库文件，请在执行还原前确保已备份重要数据。
> 目标数据库必须已存在，代码不会自动创建数据库。

### 2.1 还原到指定数据库

通过 `RestoreOptions.TargetDatabaseName` 指定目标数据库名。若未指定，则从备份文件名中自动提取（格式：`{dbname}_{timestamp}.sql`）。

```bash
mysql -h hostname -P port -u username -ppassword database_name < /backup/database_name_20260415_150405.sql
```

> **注意**：代码不会自动执行 `CREATE DATABASE`，如需还原到新数据库，请先手动创建：
>
> ```bash
> mysql -h hostname -P port -u username -ppassword -e "CREATE DATABASE IF NOT EXISTS new_database_name;"
> mysql -h hostname -P port -u username -ppassword new_database_name < /backup/database_name_20260415_150405.sql
> ```

---

## 3. Go 代码与底层命令映射

> **🔗 代码映射指南**
>
> 本部分列出了 `MySQLBackup` 逻辑备份实现中的 Go 方法与底层 mysql/mysqldump 命令的对应关系。

### 备份方法

| Go 方法 | 签名 | 对应的底层命令 |
| --- | --- | --- |
| `backupSingleDatabaseLogical()` | `(ctx context.Context, backupDir, databaseName string, callback ProgressCallback) (*BackupResult, error)` | `mysqldump -h <host> -P <port> -u <user> -p<password> --single-transaction --quick --lock-tables=false <database>` |
| `backupMultipleDatabasesLogical()` | `(ctx context.Context, backupDir string, databases []string, callback ProgressCallback) (*BackupResult, error)` | 循环调用 `backupSingleDatabaseLogical()` 备份多个数据库，单个失败不中断 |
| `backupAllDatabasesLogical()` | `(ctx context.Context, backupDir string, callback ProgressCallback) (*BackupResult, error)` | 调用 `getAllDatabases()` 获取列表后委托给 `backupMultipleDatabasesLogical()` |

### 还原方法

| Go 方法 | 签名 | 对应的底层命令 |
| --- | --- | --- |
| `restoreLogical()` | `(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error)` | `mysql -h <host> -P <port> -u <user> -p<password> <database> < <backup_file>` |

### 辅助方法

| Go 方法 | 签名 | 说明 |
| --- | --- | --- |
| `getAllDatabases()` | `(ctx context.Context) ([]string, error)` | 执行 `mysql -e "SHOW DATABASES"`，排除系统数据库（information_schema、mysql、performance_schema、sys） |
| `buildConnectionArgs()` | `() []string` | 构建 `-h <host> -P <port> -u <user> -p<password>` 连接参数 |
| `buildDumpCommandArgs()` | `() []string` | 在连接参数基础上追加 `--single-transaction --quick --lock-tables=false` |
| `execSQL()` | `(ctx context.Context, sqlStatement string) (string, error)` | 执行 `mysql -e <sql>` 命令 |
| `execMySQLDump()` | `(ctx context.Context, args []string, outputFile string) error` | 执行 `mysqldump` 命令，输出直接写入文件 |
| `execMySQLFromFile()` | `(ctx context.Context, databaseName string, inputFile io.Reader) (string, error)` | 执行 `mysql <database>` 命令，从文件读取输入 |

---

## 4. 配置项

通过 `DBConfig.Extra` 传入的配置项：

| 配置键 | 说明 | 默认值 |
| --- | --- | --- |
| `MYSQL_BIN_PATH` | MySQL 二进制文件目录（包含 mysql 和 mysqldump） | 系统 PATH 中的 `mysql` / `mysqldump` |

---

## 5. 常用辅助查询

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

### 6.1 备份时提示权限不足

```sql
GRANT ALL PRIVILEGES ON database_name.* TO 'username'@'host';
FLUSH PRIVILEGES;
```

### 6.2 还原时提示数据库不存在

代码不会自动创建目标数据库，需手动创建：

```sql
CREATE DATABASE IF NOT EXISTS database_name;
```

### 6.3 mysqldump 命令未找到

确保 MySQL bin 目录已添加到 PATH 环境变量，或通过 `MYSQL_BIN_PATH` 配置项指定：

```bash
export PATH=/path/to/mysql/bin:$PATH
```

---

## 🔗 官方文档

- [MySQL mysqldump 官方文档](https://dev.mysqlserver.cn/doc/refman/8.4/en/using-mysqldump.html)
