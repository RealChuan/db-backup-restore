# PostgreSQL 命令参考

> 详细的 API 说明和各数据库对比请参阅 [API 参考](./api.md)。

---

## 备份（Backup）

### 逻辑备份

逻辑备份使用 `pg_dump` 工具，备份文件为 SQL 纯文本格式。支持单库、多库和全部数据库备份。

#### 备份单个数据库

```bash
pg_dump -h hostname -p port -U username -d database_name --clean --if-exists -F p -f /backup/database_name_20260415_150405.sql
```

#### 备份多个数据库

逐个数据库调用 `pg_dump`：

```bash
pg_dump -h hostname -p port -U username -d db1 --clean --if-exists -F p -f /backup/db1_20260415_150405.sql
pg_dump -h hostname -p port -U username -d db2 --clean --if-exists -F p -f /backup/db2_20260415_150405.sql
```

#### 备份所有数据库

先查询所有非模板、非 `postgres` 的数据库列表，再逐个调用 `pg_dump`（不使用 `pg_dumpall`）：

```bash
pg_dump -h hostname -p port -U username -d db1 --clean --if-exists -F p -f /backup/db1_20260415_150405.sql
pg_dump -h hostname -p port -U username -d db2 --clean --if-exists -F p -f /backup/db2_20260415_150405.sql
# ... 对每个数据库重复执行
```

#### 并行备份

当前实现固定使用纯文本格式（`-F p`），不支持 `-j` 并行选项。以下为 `pg_dump` 工具本身的并行能力说明：

```bash
pg_dump -h hostname -p port -U username -d database_name -F d -j 4 -f /backup/database_name_20260415_150405
```

> 并行备份要求输出格式为目录格式（`-F d`），与纯文本格式互斥。

### 物理备份

物理备份使用 `pg_basebackup`，备份整个 PostgreSQL 实例的数据目录，适用于大规模数据库的快速备份恢复。

> 物理备份需要管理员权限执行。

#### 执行物理备份

```bash
pg_basebackup -D /backup/postgresql_20260415_150405 -X stream
```

- `-D`：指定备份输出目录
- `-X stream`：在备份过程中以流方式传输 WAL 日志，确保备份一致性

#### 并行物理备份

当前实现仅使用 `-D` 和 `-X stream` 参数，不支持 `-j` 并行选项。以下为 `pg_basebackup` 工具本身的并行能力说明：

```bash
pg_basebackup -D /backup/postgresql_20260415_150405 -X stream -j 4
```

---

## 还原（Restore）

### 逻辑还原

> 还原操作会覆盖现有数据，请在执行前确保已备份重要数据。

#### 还原到原数据库

通过 stdin 管道将备份文件传递给 `psql`：

```bash
psql -h hostname -p port -U username -d database_name < /backup/database_name_20260415_150405.sql
```

#### 还原到指定数据库

指定目标数据库名时，若覆盖模式为关闭（默认），会先自动创建目标数据库：

```bash
# 创建目标数据库
createdb -h hostname -p port -U username new_database_name

# 还原到目标数据库
psql -h hostname -p port -U username -d new_database_name < /backup/database_name_20260415_150405.sql
```

> 若未指定目标数据库名，会从备份文件名中自动提取数据库名。

### 物理还原

> 物理还原会还原整个 PostgreSQL 实例，需要停止 PostgreSQL 服务。原数据目录会被重命名为 `{datadir}_old_{timestamp}` 保留，不会自动删除。

物理还原的完整流程如下，任何步骤失败都会进行回滚：

#### 1. 停止 PostgreSQL 服务

**Linux：**

```bash
pg_ctl stop -D /var/lib/postgresql/<version>/main
# 或
systemctl stop postgresql
```

**Windows：**

```bash
net stop postgresql-x64-<version>
```

#### 2. 复制备份到临时目录

先将备份复制到临时目录 `{datadir}_new_{timestamp}`，验证通过后再切换：

```bash
mkdir -p /var/lib/postgresql/<version>/main_new_20260415_150405
cp -r /backup/postgresql_20260415_150405/* /var/lib/postgresql/<version>/main_new_20260415_150405/
```

#### 3. 验证临时目录

检查临时目录是否为有效的 PostgreSQL 数据目录（必须包含 `PG_VERSION` 文件）：

```bash
ls /var/lib/postgresql/<version>/main_new_20260415_150405/PG_VERSION
```

#### 4. 切换目录

重命名旧数据目录并切换：

```bash
mv /var/lib/postgresql/<version>/main /var/lib/postgresql/<version>/main_old_20260415_150405
mv /var/lib/postgresql/<version>/main_new_20260415_150405 /var/lib/postgresql/<version>/main
```

#### 5. 设置文件权限

**Linux：**

```bash
chmod -R 755 /var/lib/postgresql/<version>/main
```

**Windows：**

```bash
icacls "C:\Program Files\PostgreSQL\<version>\data" /grant "Everyone:(OI)(CI)F" /T
```

#### 6. 启动 PostgreSQL 服务

**Linux：**

```bash
pg_ctl start -D /var/lib/postgresql/<version>/main
# 或
systemctl start postgresql
```

**Windows：**

```bash
net start postgresql-x64-<version>
```

#### 7. 等待服务就绪

循环执行 `pg_ctl status` 等待服务启动完成（最多等待 30 秒）：

```bash
pg_ctl status -D /var/lib/postgresql/<version>/main
```

#### 8. 验证备份

启动完成后验证数据库可正常访问。

---

## 列出数据库（ListDatabases）

查询所有非模板、非 `postgres` 的数据库：

```sql
SELECT datname FROM pg_database WHERE datistemplate = false;
```

结果会过滤掉 `postgres` 数据库。

---

## 验证备份（ValidateBackup）

仅物理备份支持验证，使用 `pg_verifybackup` 检查备份完整性：

```bash
pg_verifybackup /backup/postgresql_20260415_150405
```

---

## 备份管理

### 列出备份（ListBackups）

扫描备份存储目录，列出所有备份记录。按文件系统中的备份目录/文件进行枚举。

### 删除备份（DeleteBackup）

删除指定备份标识符对应的备份文件或目录。

### 获取备份详情（GetBackupInfo）

读取指定备份的元数据信息。

### 删除所有备份（DeleteAllBackups）

清空备份存储目录中的所有备份文件。

---

## 配置项

| Extra 键       | 说明                                                                                                                      | 必填         |
| -------------- | ------------------------------------------------------------------------------------------------------------------------- | ------------ |
| `PG_BIN_PATH`  | PostgreSQL 工具目录路径，设置后从此目录查找 `psql`、`pg_dump`、`pg_dumpall`、`pg_basebackup`、`pg_verifybackup`、`pg_ctl` | 否           |
| `SSLMode`      | SSL 连接模式，非空时设置 `PGSSLMODE` 环境变量                                                                             | 否           |
| `DATA_DIR`     | PostgreSQL 数据目录路径，物理还原时必需                                                                                   | 物理还原必填 |
| `SERVICE_NAME` | Windows 下 PostgreSQL 服务名称，默认为 `postgresql-x64-18`                                                                | 否           |

---

## 常用辅助查询

### 创建数据库

```sql
CREATE DATABASE database_name;
```

### 检查数据库是否存在

```sql
SELECT 1 FROM pg_database WHERE datname = 'database_name';
```

### 查看表结构

```sql
\d table_name;
```

---

## 异机恢复指南

异机恢复是将数据库从一台机器恢复到另一台机器的过程，适用于灾难恢复、系统迁移等场景。

### 前提条件

- **平台兼容性**：源机器和目标机器可以是不同平台（Windows/Linux）
- **PostgreSQL 版本**：目标数据库的 PostgreSQL 版本应与备份时的版本相同或更高
- **备份文件完整性**：确保备份文件被完整复制
- **PostgreSQL 环境**：目标机器上已正确安装 PostgreSQL 数据库软件

### 恢复步骤

1. 在目标机器上安装 PostgreSQL 数据库软件
2. 将源机器上的备份目录复制到目标机器
3. 停止目标机器上的 PostgreSQL 服务
4. 清空目标机器上的 PostgreSQL 数据目录
5. 将备份文件复制到数据目录
6. 设置正确的文件权限
7. 启动 PostgreSQL 服务

### 命令示例

```bash
# 复制备份文件
scp -r user@source:/backup/postgresql_20260415_150405 /backup/

# 停止服务并清空数据目录
systemctl stop postgresql
rm -rf /var/lib/postgresql/18/main/*

# 复制数据并设置权限
cp -r /backup/postgresql_20260415_150405/* /var/lib/postgresql/18/main/
chown -R postgres:postgres /var/lib/postgresql/18/main

# 启动服务
systemctl start postgresql
```

### 注意事项

- 确保目标机器的 PostgreSQL 数据目录配置正确
- 确保目标数据库的字符集与源数据库一致
- 恢复前检查备份文件是否完整
- 确保执行备份的用户对备份目录和数据目录有足够的权限
- 确保目标机器的 `pg_hba.conf` 配置允许连接

---

## 故障处理

### 备份时提示权限不足

```sql
GRANT ALL PRIVILEGES ON DATABASE database_name TO username;
```

### 还原时提示数据库不存在

```sql
CREATE DATABASE database_name;
```

### pg_dump / pg_basebackup 命令未找到

确保 PostgreSQL bin 目录已添加到 PATH 环境变量：

```bash
export PATH=/path/to/postgresql/bin:$PATH
```

或通过 `PG_BIN_PATH` 配置项指定工具目录路径。

### 连接被拒绝

检查 PostgreSQL 服务是否启动，并确保监听地址正确：

```bash
systemctl status postgresql
cat /var/lib/postgresql/<version>/main/postgresql.conf | grep listen_addresses
```

### 物理还原权限不足

以管理员身份运行命令：

```bash
sudo pg_basebackup -D /backup/postgresql_20260415_150405 -X stream
```

### PostgreSQL 服务无法停止/启动

检查服务状态并查看日志：

```bash
systemctl status postgresql
tail -f /var/log/postgresql/postgresql-<version>-main.log
```

---

## 官方文档

- [PostgreSQL 备份与恢复指南](https://www.postgresql.org/docs/current/backup.html)
- [PostgreSQL pg_dump 官方文档](https://www.postgresql.org/docs/current/backup-dump.html)
