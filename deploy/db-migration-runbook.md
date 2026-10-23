# 数据库迁移操作手册：MySQL → PostgreSQL

> 适用场景：生产环境（`docker-compose.prod.yml`）数据库从 MySQL 8.0 切换到 PostgreSQL 16
> （容器自建或云数据库）。
> 迁移方式：维护窗口内短暂停机（停服务 → 备份 → 建表 → 搬数据 → 校验 → 切流），预计停机 **分钟级**
> （耗时主要取决于数据量与 pgloader 转换速度）。
> 应用代码不改方言相关逻辑：表结构由 GORM `AutoMigrate` 按 PostgreSQL 重建，数据由 pgloader 只搬行。

## 0. 迁移原理与回滚兜底

```text
迁移前：  gateway → 三个后端服务 → tcp(mysql:3306) peak-mysql 容器（mysql-data 卷）
维护窗口：停后端 → mysqldump 最终备份 → 启动 PostgreSQL → AutoMigrate 建表
          → pgloader 只迁数据 → 逐表行数校验 → 改 .env → 重启
迁移后：  gateway → 三个后端服务 → tcp(postgres:5432) peak-postgres 容器（postgres-data 卷）
          旧 MySQL 容器与 mysql-data 卷原样保留（观察期 ≥ 7 天），可随时切回
```

涉及资产：

| 文件 | 作用 |
| --- | --- |
| `scripts/db-migration/backup.sh` | PostgreSQL 一致性导出（`pg_dump -Fc`）+ 完整性校验 |
| `scripts/db-migration/restore.sh` | 目标库预检（连通性/编码）+ `pg_restore` 导入 |
| `scripts/db-migration/verify.sh` | PostgreSQL 源/目标逐表行数与内容指纹比对，差异非零退出 |
| `docker-compose.prod.yml` | `DB_DSN` 已参数化（`POSTGRES_*` 组合），自建 postgres 归入 `self-hosted-db` profile |
| `deploy/.env.production`（主机上） | 通过 `COMPOSE_PROFILES` + `POSTGRES_*` 变量切换「容器自建 / 云数据库」模式 |
| `pgloader`（一次性工具） | MySQL → PostgreSQL 只搬数据（表结构仍由 GORM 建，保证与模型一致） |

应用侧连接链路（无需改代码）：`docker-compose.prod.yml` 注入 `DB_DSN` → `apps/*/config.yaml`
的 `database.dsn` → `libs/domain/db.go` GORM 连接。启动时 `AutoMigrate` 会确保表结构存在。

---

## 1. T-1 天：预检（不停机）

在部署主机上逐项确认：

- [ ] **PostgreSQL 实例就绪**：版本 16（与应用镜像 `postgres:16-alpine` 一致）。验证：
  ```bash
  docker run --rm postgres:16-alpine psql "host=<PG_HOST> port=5432 user=<USER> dbname=postgres" -c "SELECT 1;"
  ```
- [ ] **pgloader 可用**（跨引擎搬数据）：
  ```bash
  docker run --rm dimitri/pgloader:latest pgloader --version
  ```
- [ ] **网络白名单**：PostgreSQL 已放行部署主机出口 IP（公网）或同 VPC 内网互通。
- [ ] **账号权限**：目标账号对库 `peak` 具备全部 DDL/DML 权限。GORM 启动 AutoMigrate 需要 DDL 权限。
- [ ] **目标库已创建**（云数据库账号通常无建库权限）：控制台创建库 `peak`，编码 `UTF8`。
- [ ] **时区一致**：DSN 通过 `TimeZone=Asia/Shanghai` 指定会话时区；建议实例 `timezone` 同为 `Asia/Shanghai`。
- [ ] **连接数**：实例 `max_connections` ≥ 25×3（三个服务连接池默认 `max_open_conns=25`）+ 运维余量。
- [ ] **磁盘空间**：部署主机剩余空间 ≥ 当前库大小 × 2（MySQL 备份 + PostgreSQL 数据）。
- [ ] **记录旧库基线行数**（迁移后逐表比对用）：
  ```bash
  for t in users app_clients questions mistakes images categories question_categories recognition_tasks poems; do
    n=$(docker exec -e MYSQL_PWD="$MYSQL_ROOT_PASSWORD" peak-mysql \
      sh -c "mysql -uroot -N -e 'SELECT COUNT(*) FROM \`$t\`;' peak")
    echo "$t $n"
  done | tee /tmp/mysql-baseline.txt
  ```
- [ ] **演练（强烈建议）**：用开发环境或临时容器完整跑一遍第 2 节，记录各步耗时。

## 2. 维护窗口：切换 9 步

窗口开始前公告停机时间。以下命令均在部署主机、`docker-compose.prod.yml` 所在目录执行。

### Step 1 停止后端服务（保留旧库容器）

```bash
docker compose -f docker-compose.prod.yml stop gateway question-service recognition-service user-service poetry-service web
```

预期：`docker compose ps` 仅剩 mysql（及可选观测栈）。此后数据库无增量写入。

### Step 2 旧库最终备份（回滚兜底）

```bash
mkdir -p db-backups
docker exec -e MYSQL_PWD="$MYSQL_ROOT_PASSWORD" peak-mysql \
  sh -c 'exec mysqldump --user=root --single-transaction --routines --triggers \
         --set-gtid-purged=OFF --default-character-set=utf8mb4 peak' \
  | gzip > "db-backups/mysql_peak_$(date +%Y%m%d_%H%M%S).sql.gz"
ls -lh db-backups/
```

**此备份是回滚兜底，务必留存**，并记录 `sha256sum`。

### Step 3 启动 PostgreSQL

`.env.production` 保持自建模式（或改为云数据库模式，见 Step 7）：

```dotenv
COMPOSE_PROFILES=self-hosted-db
POSTGRES_USER=peak
POSTGRES_PASSWORD=<强密码>
POSTGRES_DB=peak
```

```bash
docker compose --env-file .env.production -f docker-compose.prod.yml up -d postgres
docker compose --env-file .env.production -f docker-compose.prod.yml ps postgres   # 等待 healthy
```

### Step 4 用 GORM 建表（schema 以模型为准）

只启动 `user-service` 即可：它的启动流程会执行 `domain.Migrate`，迁移**全部**模型表。

```bash
docker compose --env-file .env.production -f docker-compose.prod.yml up -d user-service
docker logs -f peak-user-service   # 看到 "server exit" 之前的启动日志后 Ctrl-C
docker compose --env-file .env.production -f docker-compose.prod.yml stop user-service  # 建表后立即停，避免写入
```

校验表清单（应包含 `users`/`app_clients`/`questions`/`mistakes`/`images`/`categories`/
`question_categories`/`recognition_tasks`/`poems`）：

```bash
docker exec peak-postgres psql -U peak -d peak -c "\dt"
```

### Step 5 pgloader 只迁数据

> 表结构已由 GORM 建好，这里只搬行（`data only` + `truncate`），避免 pgloader 生成的类型与模型不一致。
> 网络：pgloader 容器需同时访问 MySQL 与 PostgreSQL。两者都在 compose 网络内时，把 pgloader 加入同一网络；
> 或把 `.env` 的端口临时映射到主机后用 `--network host`。

```bash
docker run --rm --network host dimitri/pgloader:latest \
  pgloader --verbose \
    --with "data only" --with "truncate" \
    "mysql://root:${MYSQL_ROOT_PASSWORD}@127.0.0.1:3306/peak" \
    "postgresql://peak:${POSTGRES_PASSWORD}@127.0.0.1:5432/peak"
```

注意事项：

- **零日期**：MySQL 的 `0000-00-00` 无法转为 PostgreSQL 时间类型，先在旧库修正：
  ```sql
  UPDATE mistakes SET recorded_at = NOW() WHERE recorded_at = '0000-00-00';
  ```
- **JSON 空字符串**：`questions.image` / `knowledge_points` 若存在空字符串，改为 `[]` / `NULL`。
- **外键与顺序**：pgloader 按表加载并处理依赖；`question_categories` 等关联表在父表之后加载。

### Step 6 重置自增序列（PostgreSQL 必需）

`data only` 不会同步序列，不重置会在首次写入时主键冲突：

```bash
docker exec -i peak-postgres psql -U peak -d peak <<'SQL'
DO $$
DECLARE r record;
BEGIN
  FOR r IN
    SELECT t.table_name, c.column_name
    FROM information_schema.tables t
    JOIN information_schema.columns c
      ON c.table_schema = t.table_schema AND c.table_name = t.table_name
    WHERE t.table_schema = 'public' AND t.table_type = 'BASE TABLE'
      AND c.column_default LIKE 'nextval(%'
  LOOP
    EXECUTE format(
      'SELECT setval(pg_get_serial_sequence(%L, %L), COALESCE((SELECT MAX(%I) FROM %I), 0) + 1, false)',
      r.table_name, r.column_name, r.column_name, r.table_name);
  END LOOP;
END $$;
SQL
```

### Step 7 校验数据

逐表行数比对（旧 MySQL vs 新 PostgreSQL）：

```bash
TABLES=$(docker exec peak-postgres psql -U peak -d peak -tAc \
  "SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY 1;")
while IFS= read -r t; do
  [[ -z "$t" ]] && continue
  src=$(docker exec -e MYSQL_PWD="$MYSQL_ROOT_PASSWORD" peak-mysql \
    sh -c "mysql -uroot -N -e 'SELECT COUNT(*) FROM \`$t\`;' peak")
  dst=$(docker exec peak-postgres psql -U peak -d peak -tAc "SELECT COUNT(*) FROM \"$t\";")
  if [[ "$src" == "$dst" ]]; then echo "✓ $t rows=$src"; else echo "✗ $t src=$src dst=$dst"; fi
done <<< "$TABLES"
```

预期：全部 `✓`。若有差异先排查再切流。**PostgreSQL 之间**的迁移/回滚复核可改用 `scripts/db-migration/verify.sh`。

### Step 8 切换 .env 并重启全部服务

```dotenv
# 容器自建 PostgreSQL（推荐原样保留）
COMPOSE_PROFILES=self-hosted-db
POSTGRES_USER=peak
POSTGRES_PASSWORD=<强密码>
POSTGRES_DB=peak

# 云数据库：注释掉 COMPOSE_PROFILES，改为指向实例
# COMPOSE_PROFILES=
# POSTGRES_HOST=pgm-xxxxxxxx.pg.rds.aliyuncs.com
# POSTGRES_PORT=5432
# POSTGRES_SSLMODE=require
```

```bash
docker compose --env-file .env.production -f docker-compose.prod.yml \
  --profile mistake --profile poetry up -d
```

预期：后端服务全部启动；`docker compose ps` 中 `peak-postgres` 为 healthy。
旧 MySQL 容器只停止、不删除（回滚兜底）：

```bash
docker stop peak-mysql
```

冒烟验证：

```bash
curl -fsS http://localhost:8080/healthz
docker exec peak-gateway wget -qO- http://question-service:8081/healthz
docker exec peak-gateway wget -qO- http://recognition-service:8082/healthz
docker exec peak-gateway wget -qO- http://user-service:8083/healthz
docker exec peak-gateway wget -qO- http://poetry-service:8084/healthz
```

- [ ] 前端可打开、能登录（`users` 表数据正常）
- [ ] 核心业务走查：题目列表、错题本（`questions`/`mistakes`）、图片识别任务历史（`images`/`recognition_tasks`）、古诗文列表（`poems`）
- [ ] **写入验证**：新建分类/错题，确认成功且重启后不丢（验证序列已重置、写入已落到 PostgreSQL）
- [ ] 观察 Grafana（observability profile）无异常报错、慢查询

### Step 9 结束窗口

确认全部通过后恢复对外服务，记录实际停机时长与窗口内问题。

## 3. 观察期与清理

- **观察期 ≥ 7 天**：期间保留 `peak-mysql` 容器（stopped）与 `mysql-data` 卷、`db-backups/` 下的最终备份。
- 观察期内重点盯：PostgreSQL 连接数/CPU/慢查询、业务侧数据完整性反馈。
- 观察期满且无异常后清理：
  ```bash
  docker rm peak-mysql
  docker volume rm <compose项目前缀>_mysql-data
  docker rmi mysql:8.0          # 可选
  # db-backups 按公司留存策略归档或删除
  ```

## 4. 回滚（切流后发现问题）

前提：旧容器/卷未删除，`.env.production` 旧值仍可查（切流前建议 `cp .env.production .env.production.bak-<日期>`）。

1. 停止全部服务：
   ```bash
   docker compose --env-file .env.production -f docker-compose.prod.yml down
   ```
2. 还原 `.env.production` 旧版本（即 MySQL 时代的配置，镜像 tag 回退到旧 commit sha）。
3. 启动：
   ```bash
   docker compose --env-file .env.production -f docker-compose.prod.yml \
     --profile mistake --profile poetry up -d
   ```
   > 回滚意味着**回到 MySQL 版本的应用镜像**（旧镜像仍指向 MySQL），因此需与 `IMAGE_TAG` 一并回退。
4. 重复 Step 8 的冒烟验证。

**重要**：若回滚发生在「应用已向 PostgreSQL 写入新数据」之后，旧库缺这段增量，
需要按 `id`/`created_at` 判断增量并反向同步，或直接以 PostgreSQL 为准重新导入。
因此**发现异常应尽早回滚**。

## 5. 常见故障处理

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| pgloader 报 `date/time field value out of range` | MySQL 零日期 `0000-00-00` | 先在旧库 UPDATE 为合法时间后重跑 |
| 首次写入报 `duplicate key value violates unique constraint` | 自增序列未重置 | 重跑 Step 6 的 `setval` 脚本 |
| 写入报 `invalid input syntax for type json` | JSON 列写入空字符串 | 修正存量数据为 `[]`/`NULL`；模型层 `BeforeSave` 已兜底新写入 |
| 服务启动循环重启 | DSN 错误 / 实例不可达 | `docker logs peak-user-service` 看连接错误；核对 `.env` 的 `POSTGRES_HOST/USER/PASSWORD/SSLMODE` |
| 报 `relation "xxx" does not exist` | AutoMigrate 未执行或连错库 | 确认 Step 4 已跑过、`PG_DATABASE` 一致 |
| 时间字段差 8 小时 | 实例时区与 `TimeZone` 不一致 | 实例 `timezone=Asia/Shanghai`，并复核存量时间字段语义 |
| `pg_restore: error: role "xxx" does not exist` | 备份含原实例属主 | 使用 `--no-owner --no-privileges`（脚本已默认） |
| 云数据库连接报 SSL 相关错误 | 实例强制 TLS | 设 `POSTGRES_SSLMODE=require`（或 `verify-full` 并配置 CA） |
| 校验行数不一致 | 迁移中断/数据仍在写入 | 确认服务已停、重跑 Step 5/6/7 |

## 6. PostgreSQL 备份 / 恢复（日常运维）

迁移完成后，备份与恢复统一使用 PostgreSQL 工具链（脚本已内置）：

```bash
# 备份（自定义格式，含结构与数据）
SRC_PASSWORD=<库密码> ./scripts/db-migration/backup.sh

# 恢复到目标实例（--clean 覆盖同名对象，注意核对目标）
DST_HOST=<实例地址> DST_USER=<用户> DST_PASSWORD=<密码> \
  ./scripts/db-migration/restore.sh ./db-backups/peak_<时间戳>.dump

# PostgreSQL 源/目标逐表行数与内容指纹比对
SRC_PASSWORD=<源密码> DST_HOST=<目标地址> DST_USER=<用户> DST_PASSWORD=<密码> \
  ./scripts/db-migration/verify.sh
```

建议用 cron/systemd timer 按日备份，并把 `db-backups/` 同步到对象存储或异地留存。
