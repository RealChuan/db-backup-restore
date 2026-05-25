# PostgreSQL 逻辑备份与还原命令手册

本文档基于 `backup` 包中的 `PostgreSQLBackup` 逻辑备份实现，汇总了逻辑备份、还原及相关操作的底层 **pg_dump** 和 **psql** 命令。

---

## 📋 目录

- [PostgreSQL 逻辑备份与还原命令手册](#postgresql-逻辑备份与还原命令手册)
  - [📋 目录](#-目录)
  - [1. 逻辑备份命令](#1-逻辑备份命令)
    - [1.1 备份单个数据库](#11-备份单个数据库)
    - [1.2 备份多个数据库](#12-备份多个数据库)
    - [1.3 备份所有数据库](#13-备份所有数据库)
    - [1.4 并行备份](#14-并行备份)
  - [2. 逻辑还原命令](#2-逻辑还原命令)
    - [2.1 还原到原数据库](#21-还原到原数据库)
    - [2.2 还原到新数据库](#22-还原到新数据库)
  - [3. Go 代码与底层命令映射](#3-go-代码与底层命令映射)
  - [4. 常用辅助查询](#4-常用辅助查询)
    - [4.1 查看数据库列表](#41-查看数据库列表)
    - [4.2 创建数据库](#42-创建数据库)
    - [4.3 检查数据库是否存在](#43-检查数据库是否存在)
    - [4.4 查看表结构](#44-查看表结构)
  - [5. 典型故障处理](#5-典型故障处理)
    - [5.1 备份时提示权限不足](#51-备份时提示权限不足)
    - [5.2 还原时提示数据库不存在](#52-还原时提示数据库不存在)
    - [5.3 pg\_dump 命令未找到](#53-pg_dump-命令未找到)
    - [5.4 连接被拒绝](#54-连接被拒绝)
  - [🔗 官方文档](#-官方文档)

---

## 1. 逻辑备份命令

PostgreSQL 逻辑备份使用 `pg_dump` 工具，备份文件为 SQL 格式。

### 1.1 备份单个数据库

```bash
pg_dump -h hostname -p port -U username -d database_name --clean --if-exists -f /backup/database_name_20260415_150405.sql
```

### 1.2 备份多个数据库

```bash
pg_dump -h hostname -p port -U username -d db1 --clean --if-exists -f /backup/db1_20260415_150405.sql
pg_dump -h hostname -p port -U username -d db2 --clean --if-exists -f /backup/db2_20260415_150405.sql
```

### 1.3 备份所有数据库

代码实现为：先通过 `getAllDatabases()` 获取所有非模板、非 `postgres` 的数据库列表，再逐个调用 `pg_dump` 备份（而非使用 `pg_dumpall`）：

```bash
# 实际执行方式：逐个数据库调用 pg_dump
pg_dump -h hostname -p port -U username -d db1 --clean --if-exists -F p -f /backup/db1_20260415_150405.sql
pg_dump -h hostname -p port -U username -d db2 --clean --if-exists -F p -f /backup/db2_20260415_150405.sql
# ...
```

> **注意**：代码中 `pgDumpallPath` 字段虽已定义，但 `backupAllDatabasesLogical()` 实际未使用 `pg_dumpall`，而是逐库调用 `pg_dump`。

### 1.4 并行备份

> **注意**：当前代码的 `backupSingleDatabaseLogical()` 固定使用 `-F p`（纯文本格式），不支持 `-j` 并行选项。以下命令仅为 `pg_dump` 工具本身的能力说明，代码中尚未实现。

```bash
pg_dump -h hostname -p port -U username -d database_name -F d -j 4 -f /backup/database_name_20260415_150405
```

---

## 2. 逻辑还原命令

> **🔄 还原注意事项**
>
> 还原操作会覆盖现有的数据库文件，请在执行还原前确保已备份重要数据。

### 2.1 还原到原数据库

代码通过 `execPsqlFromFile()` 将备份文件内容通过 stdin 管道传递给 psql（而非 `-f` 参数）：

```bash
# 代码实际执行方式：通过 stdin 管道
psql -h hostname -p port -U username -d database_name < /backup/database_name_20260415_150405.sql
```

### 2.2 还原到新数据库

代码通过 `RestoreOptions.TargetDatabaseName` 指定目标数据库名，通过 `RestoreOptions.Overwrite` 控制是否跳过自动创建数据库：

```bash
# 若 Overwrite 为 false（默认），代码会先调用 createDatabaseIfNotExists() 创建数据库
createdb -h hostname -p port -U username new_database_name

# 然后通过 stdin 管道还原
psql -h hostname -p port -U username -d new_database_name < /backup/database_name_20260415_150405.sql
```

> **注意**：若未指定 `TargetDatabaseName`，代码会通过 `ExtractDatabaseName()` 从备份文件名中自动提取数据库名。

---

## 3. Go 代码与底层命令映射

> **🔗 代码映射指南**
>
> 本部分列出了 `PostgreSQLBackup` 逻辑备份实现中的 Go 方法与底层 pg_dump/psql 命令的对应关系。

| Go 方法 | 对应的底层命令 |
| --- | --- |
| `backupSingleDatabaseLogical()` | `pg_dump -F p --clean --if-exists -d <database> -f <path>` |
| `backupMultipleDatabasesLogical()` | 循环调用 `backupSingleDatabaseLogical()` 备份多个数据库 |
| `backupAllDatabasesLogical()` | 调用 `getAllDatabases()` 获取数据库列表后，委托给 `backupMultipleDatabasesLogical()` |
| `restoreLogical()` | `psql -d <database>` 通过 stdin 管道读入备份文件（非 `-f` 参数） |
| `getAllDatabases()` | `psql -c "SELECT datname FROM pg_database WHERE datistemplate = false;"`，并过滤掉 `postgres` 数据库 |
| `createDatabaseIfNotExists()` | 先查询 `SELECT 1 FROM pg_database WHERE datname = '<name>'`，不存在则执行 `CREATE DATABASE "<name>"` |
| `execSQL()` | `psql -c <sql>` 执行 SQL 命令（内部辅助方法） |
| `execPgDump()` | 执行 `pg_dump` 命令，支持纯文本和目录输出格式（内部辅助方法） |
| `execPsqlFromFile()` | `psql -d <database>` 通过 stdin 管道读入文件内容（内部辅助方法） |

---

## 4. 常用辅助查询

### 4.1 查看数据库列表

```sql
SELECT datname FROM pg_database WHERE datistemplate = false;
```

### 4.2 创建数据库

```sql
CREATE DATABASE database_name;
```

### 4.3 检查数据库是否存在

```sql
SELECT 1 FROM pg_database WHERE datname = 'database_name';
```

### 4.4 查看表结构

```sql
\d table_name;
```

---

## 5. 典型故障处理

### 5.1 备份时提示权限不足

```sql
GRANT ALL PRIVILEGES ON DATABASE database_name TO username;
```

### 5.2 还原时提示数据库不存在

```sql
CREATE DATABASE database_name;
```

### 5.3 pg_dump 命令未找到

确保 PostgreSQL bin 目录已添加到 PATH 环境变量：

```bash
export PATH=/path/to/postgresql/bin:$PATH
```

### 5.4 连接被拒绝

检查 PostgreSQL 服务是否启动，并确保监听地址正确：

```bash
systemctl status postgresql
cat /var/lib/postgresql/<version>/main/postgresql.conf | grep listen_addresses
```

---

## 🔗 官方文档

- [PostgreSQL pg_dump 官方文档](https://www.postgresql.org/docs/current/backup-dump.html)
