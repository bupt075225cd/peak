# 数据库迁移操作手册：自建 MySQL 容器 → 云 RDS

> 适用场景：生产环境（`docker-compose.prod.yml`）数据库从单机容器自建 MySQL 8.0 迁移到云 RDS（或其他主机上的 MySQL 8.0）。
> 迁移方式：维护窗口内短暂停机（停服务 → 导出 → 导入 → 校验 → 切换重启），预计停机 **分钟级**。
> 引擎不变（MySQL），应用代码零改动，仅切换编排配置。

## 0. 迁移原理与回滚兜底

```text
迁移前：  gateway → 三个后端服务 → tcp(mysql:3306) peak-mysql 容器（mysql-data 卷）
维护窗口：停后端 → mysqldump 一致性导出 → 导入 RDS → 逐表校验 → 改 .env → 重启
迁移后：  gateway → 三个后端服务 → tcp(RDS_HOST:3306) 云 RDS
          旧容器与 mysql-data 卷原样保留（观察期 ≥ 7 天），可随时切回
```

涉及资产：

| 文件 | 作用 |
| --- | --- |
| `scripts/db-migration/backup.sh` | 源库一致性导出 + gzip + 完整性校验 |
| `scripts/db-migration/restore.sh` | 目标库预检（连通性/字符集）+ 导入 |
| `scripts/db-migration/verify.sh` | 源/目标逐表行数 + CHECKSUM 比对，差异非零退出 |
| `docker-compose.prod.yml` | `DB_DSN` 已参数化（`MYSQL_HOST`/`MYSQL_PORT` 等组合），自建 mysql 容器归入 `self-hosted-db` profile |
| `deploy.env`（主机上） | 通过 `COMPOSE_PROFILES` + `MYSQL_*` 变量切换「容器自建 / RDS」模式 |

应用侧连接链路（无需改动）：`docker-compose.prod.yml` 注入 `DB_DSN` → `apps/*/config.yaml` 的 `database.dsn` → `libs/domain/db.go` GORM 连接。启动时 GORM `AutoMigrate` 会确保表结构存在，故导入的数据可被直接使用。

---

## 1. T-1 天：预检（不停机）

在部署主机上逐项确认：

- [ ] **RDS 实例就绪**：版本 MySQL 8.0（与应用 `mysql:8.0` 一致），规格满足业务。
- [ ] **网络白名单**：RDS 已放行部署主机的出口 IP（公网）或同 VPC 内网互通。验证：
  ```bash
  docker run --rm mysql:8.0 mysql -h <RDS_HOST> -P 3306 -u <USER> -p -e "SELECT 1;"
  ```
- [ ] **账号权限**：目标账号对库 `peak` 具备全部 DDL/DML 权限（SELECT/INSERT/UPDATE/DELETE/CREATE/ALTER/INDEX/DROP/REFERENCES）。GORM 启动 AutoMigrate 需要 DDL 权限。
- [ ] **目标库已创建**（RDS 账号通常无建库权限）：控制台创建库 `peak`，字符集 `utf8mb4`、排序规则 `utf8mb4_unicode_ci`。
- [ ] **时区一致**：DSN 使用 `loc=Local`，请把 RDS 参数 `time_zone` 设置为与部署主机相同的时区，避免时间字段偏移。
- [ ] **连接数**：RDS `max_connections` ≥ 25×3（三个服务连接池默认 `max_open_conns=25`）+ 运维余量。
- [ ] **磁盘空间**：部署主机剩余空间 ≥ 当前库大小 × 2（备份 gz + 解压导入缓冲）。查看当前库大小：
  ```bash
  docker exec peak-mysql sh -c 'mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -N -e \
    "SELECT ROUND(SUM(data_length+index_length)/1024/1024,1) AS mb FROM information_schema.tables WHERE table_schema=\"peak\";"'
  ```
- [ ] **发布编排就绪**：确认 `docker-compose.prod.yml` 已更新为参数化版本（`DB_DSN` 含 `MYSQL_HOST` 等），且已推送到部署主机。
- [ ] **在 `.env` 中补充当前自建库连接变量**（切流前先与现网保持一致，验证参数化无损）：
  ```dotenv
  COMPOSE_PROFILES=self-hosted-db
  MYSQL_ROOT_PASSWORD=<现网 root 密码>
  MYSQL_DATABASE=peak
  ```
  然后执行 `docker compose -f docker-compose.prod.yml up -d` 确认服务照常启动（回归验证）。
- [ ] **演练（强烈建议）**：用开发环境或临时容器完整跑一遍第 2~4 节，记录各步耗时，据此评估维护窗口长度。

## 2. 维护窗口：切换 8 步

窗口开始前公告用户停机时间。以下命令均在部署主机、`docker-compose.prod.yml` 所在目录执行。

### Step 1 停止后端服务（保留旧库容器）

```bash
docker compose -f docker-compose.prod.yml stop gateway question-service recognition-service user-service web
```

预期：`docker compose ps` 仅剩 mysql（及可选观测栈）。此后数据库无增量写入。

### Step 2 最终备份

```bash
SRC_PASSWORD=<现网 root 密码> ./scripts/db-migration/backup.sh
```

预期输出 `备份完成：./db-backups/peak_<时间戳>.sql.gz`，并记录末尾提示的 sha256。**此备份同时是回滚兜底，务必留存。**

### Step 3 导入 RDS

```bash
DST_HOST=<RDS_HOST> DST_USER=<USER> DST_PASSWORD=<PASSWORD> \
  ./scripts/db-migration/restore.sh ./db-backups/peak_<时间戳>.sql.gz
```

脚本会先做连通性与字符集预检再导入；失败时按报错处理（见第 5 节）后重跑。

### Step 4 校验数据

```bash
SRC_PASSWORD=<现网 root 密码> \
DST_HOST=<RDS_HOST> DST_USER=<USER> DST_PASSWORD=<PASSWORD> \
  ./scripts/db-migration/verify.sh
```

预期：所有表 `rows` 一致、`checksum 一致`，退出码 0。
说明：若个别表 `rows` 相同但 checksum 不一致，多为字符集/排序规则差异所致，可对该表抽样比对关键字段；`rows` 不一致则必须重新导入。

### Step 5 切换 .env 到 RDS 模式

修改主机上的 `.env`：

```dotenv
# COMPOSE_PROFILES=            ← 删除或置空（不再启动本地 mysql 容器）
MYSQL_HOST=<RDS_HOST>
MYSQL_PORT=3306
MYSQL_USER=<USER>
MYSQL_ROOT_PASSWORD=<PASSWORD>   # 此处含义为「应用连接数据库的密码」
MYSQL_DATABASE=peak
```

### Step 6 重启全部服务

```bash
docker compose -f docker-compose.prod.yml up -d
```

预期：`up` 输出不再包含 peak-mysql；三个后端服务与 gateway、web 全部启动。
⚠️ 旧容器如仍在运行（切换前未删除），不要 `rm`，只停止保留回滚兜底：
```bash
docker stop peak-mysql   # 保留容器与 mysql-data 卷，回滚兜底
```

### Step 7 冒烟验证

```bash
# 1) 健康检查
curl -fsS http://localhost:8080/healthz
# 内网服务健康检查（主机不可直达时在容器内验证）：
docker exec peak-gateway wget -qO- http://question-service:8081/healthz
docker exec peak-gateway wget -qO- http://recognition-service:8082/healthz
docker exec peak-gateway wget -qO- http://user-service:8083/healthz
```

- [ ] 前端页面可打开、能登录（`users` 表数据正常）
- [ ] 核心业务走查：题目列表、错题本（`questions`/`mistakes`）、图片识别任务历史（`images`/`recognition_tasks`）
- [ ] **写入验证**：在页面执行一次会产生写库的操作（如新建分类），确认操作成功且重启后不丢——即确认应用已写入 RDS
- [ ] 观察 Grafana（observability profile）无异常报错、慢查询

### Step 8 结束窗口

确认全部通过后恢复对外服务，记录实际停机时长与窗口内问题。

## 3. 观察期与清理

- **观察期 ≥ 7 天**：期间保留 `peak-mysql` 容器（stopped）与 `mysql-data` 卷、`db-backups/` 下的最终备份。
- 观察期内重点盯：RDS 连接数/CPU/慢查询日志、业务侧数据完整性反馈。
- 观察期满且无异常后清理：
  ```bash
  docker rm peak-mysql
  docker volume rm <compose项目前缀>_mysql-data
  docker rmi mysql:8.0          # 可选
  # db-backups 按公司留存策略归档或删除
  ```
- `.env` 中确认 `COMPOSE_PROFILES` 已移除，避免误启动本地库。

## 4. 回滚（切流后发现问题）

前提：旧容器/卷未删除，`.env` 旧值仍可查（切流前建议 `cp .env .env.bak-<日期>`）。

1. 停止全部服务：
   ```bash
   docker compose -f docker-compose.prod.yml down
   ```
2. 还原 `.env` 为自建模式（`COMPOSE_PROFILES=self-hosted-db`、去掉 `MYSQL_HOST` 等）：
   ```bash
   cp .env.bak-<日期> .env
   ```
3. 重启：
   ```bash
   docker compose -f docker-compose.prod.yml up -d
   ```
4. 重复 Step 7 冒烟验证。

**重要**：若回滚发生在「应用已向 RDS 写入新数据」之后，旧库缺这段增量。处理顺序：
1) 停服务（同 Step 1）→ 2) 反向补数据：对 RDS 中新写入的表（按 `id`/`created_at` 判断增量）导出并导入旧库，或直接以 RDS 为准再次全量回灌旧库 → 3) 再执行上述回滚。
因此**发现异常应尽早回滚**，拖得越久增量越大。

## 5. 常见故障处理

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| restore.sh 预检连接失败 | 白名单/安全组未放行、地址写错 | 检查 RDS 白名单与 `DST_HOST`；建议内网互通 |
| 报 `Access denied` | 账号/密码错或权限不足 | RDS 控制台核对账号与库授权 |
| `CREATE DATABASE` 权限报错（脚本自动跳过） | RDS 账号默认无建库权限 | 控制台预建库 utf8mb4 后重跑 |
| verify.sh 目标缺表 | 导入中断/不完整 | 清空目标库（DROP 后重建）重跑 restore.sh |
| 服务启动循环重启 | DSN 错误、RDS 不可达 | `docker logs peak-user-service` 看连接错误；核对 `.env` 的 `MYSQL_HOST/USER/PASSWORD` |
| 时间字段差 8 小时 | RDS `time_zone` 与主机不一致 | RDS 参数组设 `time_zone` 后重启实例，并复核存量时间字段语义 |
| GORM 报表不存在 | 导入库名与 `MYSQL_DATABASE` 不一致 | 统一库名；或依赖 AutoMigrate 重建空表后再导入数据 |
| verify checksum 不一致但行数一致 | 字符集/排序规则差异或 UPDATE 时间戳字段 | 抽样比对关键字段，确认无误后可放行 |
