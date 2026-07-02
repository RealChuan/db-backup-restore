# MySQL 命令参考

> 详细的 API 说明和各数据库对比请参阅 [API 参考](./api.md)。

MySQL 支持两种备份方式：

- **逻辑备份**：使用 `mysqldump` 工具，适用于 InnoDB 引擎，备份文件为 SQL 格式
- **物理备份**：使用 `Percona XtraBackup`，适用于大规模数据库的快速备份恢复

---

## 备份（Backup）

### 逻辑备份

逻辑备份使用 `mysqldump` 工具，备份文件为 SQL 格式，适用于 InnoDB 引擎。

标准参数：`--single-transaction --quick --lock-tables=false`

> **注意**：逻辑备份当前不支持压缩（`--compress` / gzip）。

#### 备份单个数据库

```bash
mysqldump -h hostname -P port -u username -ppassword --single-transaction --quick --lock-tables=false database_name > /backup/database_name_20260415_150405.sql
```

#### 备份多个数据库

```bash
mysqldump -h hostname -P port -u username -ppassword --single-transaction --quick --lock-tables=false db1 > /backup/db1_20260415_150405.sql
mysqldump -h hostname -P port -u username -ppassword --single-transaction --quick --lock-tables=false db2 > /backup/db2_20260415_150405.sql
```

#### 备份所有数据库

```bash
for db in $(mysql -h hostname -P port -u username -ppassword -e "SHOW DATABASES" | grep -v -E "^Database$|information_schema|mysql|performance_schema|sys"); do
    mysqldump -h hostname -P port -u username -ppassword --single-transaction --quick --lock-tables=false $db > /backup/${db}_20260415_150405.sql
done
```

### 物理备份

物理备份需要管理员权限执行。备份过程分为两步：执行备份和准备备份（prepare），两步均需完成才算备份成功。

#### 执行物理备份

```bash
xtrabackup --backup \
  --target-dir=/backup/mysql_20260415_150405 \
  --host=hostname \
  --port=port \
  --user=username \
  --password=password
```

#### 准备备份（prepare）

```bash
xtrabackup --prepare \
  --target-dir=/backup/mysql_20260415_150405
```

> 备份操作会自动依次执行 backup 和 prepare 两步。

---

## 还原（Restore）

### 逻辑还原

还原操作会覆盖现有的数据库文件，请在执行还原前确保已备份重要数据。目标数据库必须已存在，不会自动创建数据库。

#### 还原到指定数据库

指定目标数据库名进行还原。若未指定，则从备份文件名中自动提取（格式：`{dbname}_{timestamp}.sql`）。

```bash
mysql -h hostname -P port -u username -ppassword database_name < /backup/database_name_20260415_150405.sql
```

> 如需还原到新数据库，请先手动创建：
>
> ```bash
> mysql -h hostname -P port -u username -ppassword -e "CREATE DATABASE IF NOT EXISTS new_database_name;"
> mysql -h hostname -P port -u username -ppassword new_database_name < /backup/database_name_20260415_150405.sql
> ```

### 物理还原

物理还原会还原整个 MySQL 实例，需要停止 MySQL 服务。原数据目录会被重命名为 `{datadir}_old_{timestamp}` 保留，不会自动删除。

采用"临时目录策略"最小化停机时间：先将备份还原到临时目录并验证，确认无误后才停止服务并切换目录。

完整流程：创建临时目录 → 还原到临时目录 → 验证临时目录 → 停止服务 → 重命名旧目录 → 切换新目录 → 设置权限 → 启动服务

#### 1. 创建临时目录

```bash
mkdir -p /var/lib/mysql_new_20260415_150405
```

#### 2. 还原到临时目录（copy-back）

```bash
xtrabackup --copy-back \
  --src-dir=/backup/mysql_20260415_150405 \
  --datadir=/var/lib/mysql_new_20260415_150405
```

#### 3. 验证临时目录

验证数据目录包含 MySQL 特征文件（`ibdata1` 或 `mysql/` 目录），确保还原数据有效。

#### 4. 停止 MySQL 服务

> 停止服务在还原到临时目录并验证之后执行，以最小化停机时间。

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

#### 5. 重命名旧数据目录并切换

```bash
# 重命名旧数据目录（保留备份）
mv /var/lib/mysql /var/lib/mysql_old_20260415_150405

# 切换到新数据目录
mv /var/lib/mysql_new_20260415_150405 /var/lib/mysql
```

> 如果切换新目录失败，会将旧目录恢复原位并重启服务。

#### 6. 设置文件权限

```bash
# 递归设置所有文件和目录权限为 755
chmod -R 755 /var/lib/mysql
```

> 所有文件和目录统一设置 `0755` 权限，而非分别设置 `644`（文件）和 `755`（目录）。

#### 7. 启动 MySQL 服务

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

## 列出数据库（ListDatabases）

执行 `SHOW DATABASES` 并排除系统数据库：

```bash
mysql -h hostname -P port -u username -ppassword -e "SHOW DATABASES"
```

排除的系统数据库：`information_schema`、`mysql`、`performance_schema`、`sys`

等价 SQL：

```sql
SELECT SCHEMA_NAME FROM information_schema.SCHEMATA
WHERE SCHEMA_NAME NOT IN ('information_schema', 'mysql', 'performance_schema', 'sys');
```

---

## 备份管理

### 列出备份（ListBackups）

扫描备份目录，列出所有已保存的备份记录。每个备份对应备份目录下的一个子目录或文件。

### 删除备份（DeleteBackup）

从文件系统中删除指定备份的目录及其全部内容。

### 获取备份详情（GetBackupInfo）

读取备份目录中的元数据文件，返回备份的详细信息（如备份时间、数据库列表、备份类型等）。

### 删除所有备份（DeleteAllBackups）

清空整个备份目录，删除所有备份文件和子目录。

---

## 配置项

| Extra 键              | 说明                                            | 必填           | 默认值                                                            |
| --------------------- | ----------------------------------------------- | -------------- | ----------------------------------------------------------------- |
| `MYSQL_BIN_PATH`      | MySQL 二进制文件目录（包含 mysql 和 mysqldump） | 否             | 系统 PATH 中的 `mysql` / `mysqldump`                              |
| `XTRABACKUP_BIN_PATH` | XtraBackup 二进制文件目录                       | 否             | 系统 PATH 中的 `xtrabackup` 或 `innobackupex`                     |
| `SERVICE_NAME`        | MySQL 服务名（用于物理还原时停止/启动服务）     | 否             | Windows: `MySQL`；Linux: 自动检测（mysqld/mysql/mariadb/percona） |
| `DATA_DIR`            | MySQL 数据目录                                  | 物理还原时必填 | 无                                                                |

---

## 故障处理

### 逻辑备份相关

#### 备份时提示权限不足

```sql
GRANT ALL PRIVILEGES ON database_name.* TO 'username'@'host';
FLUSH PRIVILEGES;
```

#### 还原时提示数据库不存在

还原不会自动创建目标数据库，需手动创建：

```sql
CREATE DATABASE IF NOT EXISTS database_name;
```

#### mysqldump 命令未找到

确保 MySQL bin 目录已添加到 PATH 环境变量，或通过 `MYSQL_BIN_PATH` 配置项指定：

```bash
export PATH=/path/to/mysql/bin:$PATH
```

### 物理备份相关

#### xtrabackup 命令未找到

安装 Percona XtraBackup，或通过 `XTRABACKUP_BIN_PATH` 配置项指定路径：

```bash
# CentOS/RHEL
yum install percona-xtrabackup-80

# Ubuntu/Debian
apt-get install percona-xtrabackup-80
```

#### 权限不足

以管理员身份运行命令：

```bash
sudo xtrabackup --backup ...
```

#### MySQL 服务无法停止/启动

检查服务状态并排查问题：

```bash
# 检查服务状态
systemctl status mysqld

# 查看日志
tail -f /var/log/mysqld.log
```

### 异机恢复指南

异机恢复是将数据库从一台机器恢复到另一台机器的过程，适用于灾难恢复、系统迁移等场景。

#### 前提条件

- 源机器和目标机器可以是不同平台（Windows/Linux）
- 目标数据库的 MySQL 版本应与备份时的版本相同或更高
- 确保备份文件被完整复制
- 目标机器上已安装 Percona XtraBackup
- 目标机器上已正确安装 MySQL 数据库软件

#### 恢复步骤

1. 在目标机器上安装 MySQL 和 Percona XtraBackup
2. 将源机器上的备份目录复制到目标机器
3. 停止目标机器上的 MySQL 服务
4. 清空目标机器上的 MySQL 数据目录
5. 使用 `xtrabackup --copy-back` 还原数据
6. 设置正确的文件权限
7. 启动 MySQL 服务

#### 命令示例

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

#### 注意事项

- 确保目标机器的 MySQL 数据目录配置正确
- 确保目标数据库的字符集与源数据库一致
- 在恢复前检查备份文件是否完整
- 确保执行备份的用户对备份目录和数据目录有足够的权限

---

## 官方文档

- [MySQL 备份与恢复指南](https://dev.mysqlserver.cn/doc/refman/8.4/en/backup-and-recovery.html)
- [MySQL mysqldump 官方文档](https://dev.mysqlserver.cn/doc/refman/8.4/en/using-mysqldump.html)
- [Percona XtraBackup 官方文档](https://docs.percona.com/percona-xtrabackup/8.4/)
