# MariaDB 与 Redis 接入说明

## 1. 当前实现范围

本项目新增 `internal/adapters/storage/mysqlredis` 适配层：

- MariaDB：持久化分析运行、阶段事件、原始数据快照、标准化行情快照、组合快照和最终结果。
- Redis：缓存最终分析结果和快照，并提供可选的通用 cache-aside 方法。
- `Store` 实现主流程需要的 `app.RunStore` 与 `app.SnapshotStore` 接口，并提供结果查询、审计事件查询和读穿缓存；主流程无需直接依赖 SQL 或 Redis 客户端。
- `cmd/storage-check` 检查 MariaDB/Redis 连接并按配置执行迁移。

MariaDB 是持久化事实源；Redis 仅作为结果/快照及通用缓存，不应存放唯一副本，也不应把缓存命中当成数据质量验证。根目录主 API 的实时行情与日 K 现通过原生 Go market-data 服务获取；`go-core` 仍通过 Python bridge 调用尚未迁移的部分市场概况、新闻、LLM 和辅助能力。缓存不能绕过主流程的数据质量门禁。

## 2. 配置 `.env`

复制 `.env.example` 为 `.env`，填写实际地址和凭据。`.env` 已加入 `.gitignore`，不要提交真实密码。

```dotenv
DB_HOST=127.0.0.1
DB_PORT=3306
DB_USER=stock_analysis
DB_PASSWORD=请替换为数据库密码
DB_NAME=stock_analysis
DB_PARAMS=charset=utf8mb4&parseTime=true&loc=UTC
DB_MAX_OPEN_CONNS=20
DB_MAX_IDLE_CONNS=5
DB_CONN_MAX_LIFETIME=30m
DB_AUTO_MIGRATE=false

REDIS_ADDR=127.0.0.1:6379
REDIS_PASSWORD=
REDIS_DB=0
REDIS_KEY_PREFIX=stock-analysis:
REDIS_CACHE_TTL=5m
REDIS_DIAL_TIMEOUT=3s
REDIS_READ_TIMEOUT=2s
REDIS_WRITE_TIMEOUT=2s
```

注意：

- `DB_HOST` 和 `REDIS_ADDR` 必须是 Go 进程所在网络能访问的地址。若 Go 在 Docker 容器内运行，应使用 Docker 网络中的服务名，不要使用容器自身的 `127.0.0.1` 指向其他容器。
- 数据库账号需要目标库的建表、读写权限。先由管理员创建数据库，再运行迁移。
- `DB_AUTO_MIGRATE=true` 会在启动时创建缺失表；生产环境建议由发布步骤执行迁移，应用运行时设置为 `false`。
- `DB_PARAMS` 使用 MySQL DSN 查询参数格式。不要将密码放进 `DB_PARAMS`。
- `.env` 加载遵循“进程环境变量优先于文件”的规则，便于 Docker/systemd 覆盖配置。

## 3. 初始化数据库

以有权限的 MariaDB 管理员执行一次：

```sql
CREATE DATABASE stock_analysis
  CHARACTER SET utf8mb4
  COLLATE utf8mb4_unicode_ci;
CREATE USER 'stock_analysis'@'%' IDENTIFIED BY '替换为强密码';
GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, INDEX, REFERENCES
  ON stock_analysis.* TO 'stock_analysis'@'%';
FLUSH PRIVILEGES;
```

按你的网络安全策略收紧账号 Host 范围，不建议在公网开放 MariaDB 端口。若用户已存在，不要重复执行 `CREATE USER`。

## 4. 连接与迁移验证

在包含 `go.mod` 的项目根目录执行：

```bash
# 先编辑 .env；需要自动建表时设置 DB_AUTO_MIGRATE=true
go run ./cmd/storage-check
```

成功后会检查 MariaDB、必需表结构和 Redis 连接；并对 MariaDB 执行事务内插入/查询后回滚，对 Redis 执行 SET/GET/DEL 往返测试。若 `DB_AUTO_MIGRATE=true`，会创建以下表：

- `analysis_runs`：每次运行的请求、状态、最终结构化结果。
- `analysis_stage_events`：每个阶段的开始/结束/失败审计事件。
- `analysis_snapshots`：原始、标准化市场和模拟组合快照的 JSON 内容及索引元数据。

迁移使用 `CREATE TABLE IF NOT EXISTS`，可重复运行。当前没有实现自动清理历史快照；应在确认保留策略后再增加归档/清理任务。

## 5. Redis 的职责和失败策略

Redis key 使用 `REDIS_KEY_PREFIX` 统一前缀：

- `stock-analysis:run:<run_id>`：最终运行结果缓存。
- `stock-analysis:snapshot:<snapshot_id>`：快照缓存。
- `stock-analysis:cache:<key>`：供后续业务显式调用的通用缓存接口。

缓存 TTL 由 `REDIS_CACHE_TTL` 控制。任何需要审计或回放的数据都必须先写 MariaDB。不要把 Redis TTL 缓存作为唯一历史记录。

当前 `Open` 会检查 MariaDB/Redis 连通性，并验证三张必要表存在；任一项不满足时启动失败。`/readyz` 会持续检查数据库表结构与 Redis。分析完成后写入 MariaDB 是硬要求，Redis 结果/快照缓存写入失败则仅记录警告，不会回滚已持久化的数据。当前启动阶段仍要求 Redis 可连接；尚未实现 Redis 不可用时允许 Go 服务带降级运行的模式。

## 6. 代码接入边界

`mysqlredis.Store` 实现：

- `app.RunStore`：`Begin`、`AppendEvent`、`Finish`。
- `app.SnapshotStore`：`SaveRaw`、`SaveMarket`、`SavePortfolio`。

在应用装配层将同一个 `*mysqlredis.Store` 分别注入 `Dependencies.Store` 和 `Dependencies.Snapshots`。不要在 `Runner` 中直接调用 `sql.DB` 或 Redis API。

示意：

```go
storage, err := mysqlredis.Open(ctx, cfg)
if err != nil { return err }
defer storage.Close()

deps.Store = storage
deps.Snapshots = storage
runner, err := app.NewRunner(deps)
```

`cfg` 应由 `mysqlredis.LoadConfigFromEnv()` 创建；先调用 `config.LoadDotEnvIfExists(".env")` 加载配置。

## 7. 当前已接入与边界

正式入口为 `cmd/analysis-api`，已把同一个 `*mysqlredis.Store` 注入 `Dependencies.Store` 和 `Dependencies.Snapshots`，同时对外提供 `GET /readyz`、`POST /api/v1/analysis/runs`、结果查询和审计事件查询。结果查询使用 Redis 优先、MariaDB 兜底；Redis 写缓存失败会记警告，不会覆盖 MariaDB 已经保存的结果。相同 `run_id` 不允许重复 Begin，避免悄悄覆盖已有运行记录；客户端重试应查询原 `run_id`，而不是复用 ID 启动新任务。

当前快照以 JSON 持久化，适合审计和复现主流程输入，但还没有为股票行情建立供大规模历史分析使用的标准化事实表。没有历史清理任务、备份恢复自动化、完整模拟组合账本或 Redis 锁/任务队列。请在服务器上实际执行 `go run ./cmd/storage-check` 和一次 HTTP 分析请求，验证 DB 权限、服务名、迁移与缓存读写。
