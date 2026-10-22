# Go 服务部署就绪代码 Review

> Review 日期：2026-10-22  
> 目标分支：`feat/full-go-data-providers-api-switch`  
> Review 范围：Go 行情服务、Go 分析 API、Dockerfile、Compose、环境变量与首次部署流程。

## 1. Review 结论

**静态代码与 Go 自动化测试通过后，建议按本文的预检脚本进行首次部署；不能仅凭 CI 承诺真实服务器必然一次成功。** 真实部署还依赖当前服务器上的 Docker 网络、数据库是否存在、账号权限、Redis 密码和行情供应商网络。

本分支包含根 Go 分析 API 和原生 Go market-data 两个部署服务。该 Compose 文件不启动 `go-core` 或 Python bridge；当前根 API 的行情与日 K 不依赖 Python 行情 fallback。

## 2. 本轮重点核查与修正

| 项目 | Review 结果 |
|---|---|
| Go 模块测试 | 根 module、`go-core`、`services/market-data` 的格式检查、测试、race 和 vet 已在既有 CI 运行 #147 通过 |
| Docker 实际构建 | 之前 CI 没有构建两个 Dockerfile。本轮已把 Compose 语法检查和两个镜像构建加入 CI，结果以本分支最新 Actions 运行记录为准 |
| Compose 配置输入 | 两个容器共用同一个 `MARKET_DATA_API_KEY`；行情服务可使用拆分后的 `MARKET_DATA_DB_*`，旧 `MARKET_DATA_MYSQL_DSN` 暂时兼容 |
| 数据库/Redis 复用 | 两个容器连接现有 `1panel-network` 内服务，不应再建第二套数据库容器 |
| 宿主机端口安全 | 分析 API 没有内置用户认证，宿主机映射已限制为 `127.0.0.1:8092`，避免直接绑定公网网卡 |
| MariaDB 凭据 | 分析 API 使用 MySQL 驱动原生 Connector，不把密码拼入 DSN 字符串；新增测试覆盖包含 `@`、`:`、`/`、`?`、`#`、`&`、`# Go 服务部署就绪代码 Review

> Review 日期：2026-10-22  
> 目标分支：`feat/full-go-data-providers-api-switch`  
> Review 范围：Go 行情服务、Go 分析 API、Dockerfile、Compose、环境变量与首次部署流程。

## 1. Review 结论

**静态代码与 Go 自动化测试通过后，建议按本文的预检脚本进行首次部署；不能仅凭 CI 承诺真实服务器必然一次成功。** 真实部署还依赖当前服务器上的 Docker 网络、数据库是否存在、账号权限、Redis 密码和行情供应商网络。

本分支包含根 Go 分析 API 和原生 Go market-data 两个部署服务。该 Compose 文件不启动 `go-core` 或 Python bridge；当前根 API 的行情与日 K 不依赖 Python 行情 fallback。

## 2. 本轮重点核查与修正

| 项目 | Review 结果 |
|---|---|
| Go 模块测试 | 根 module、`go-core`、`services/market-data` 的格式检查、测试、race 和 vet 已在既有 CI 运行 #147 通过 |
| Docker 实际构建 | 之前 CI 没有构建两个 Dockerfile。本轮已把 Compose 语法检查和两个镜像构建加入 CI，结果以本分支最新 Actions 运行记录为准 |
| Compose 配置输入 | 两个容器共用同一个 `MARKET_DATA_API_KEY`；行情服务可使用拆分后的 `MARKET_DATA_DB_*`，旧 `MARKET_DATA_MYSQL_DSN` 暂时兼容 |
| 数据库/Redis 复用 | 两个容器连接现有 `1panel-network` 内服务，不应再建第二套数据库容器 |
、`%` 的凭据 |
| 构建上下文 | 根目录与 `services/market-data` 分别有 `.dockerignore`，避免把本地 `.env` 带入 Docker 构建上下文 |
| 部署可操作性 | 新增 `scripts/deploy-go.sh`，按顺序检查配置、构建、启动并等待健康状态；失败会输出最近日志 |

## 3. 部署前必须满足的条件

1. 已存在 Docker 网络 `1panel-network`，现有 MariaDB 与 Redis 容器已加入该网络。
2. MariaDB 数据库本身必须预先存在；自动迁移只创建表，不会自动创建数据库。
3. `.env` 中的 MariaDB 数据库名、账号、密码和服务主机名都是真实值；不能直接使用 `.env.example` 中的占位值。
4. `MARKET_DATA_API_KEY` 是不少于 32 个字符的随机密钥；两个 Go 服务必须使用同一值。
5. 如果 `DB_AUTO_MIGRATE=true` 和 `MARKET_DATA_DB_AUTO_MIGRATE=true`，对应账号必须对目标数据库有建表和变更表结构所需的权限。没有权限时服务会在启动/就绪检查阶段失败。
6. `MARKET_DATA_REDIS_ADDR` 与 `REDIS_ADDR` 应使用网络内可解析的 Redis 地址；两个变量可使用相同地址及密码，也可以单独配置。
7. 密码如果包含 `# Go 服务部署就绪代码 Review

> Review 日期：2026-10-22  
> 目标分支：`feat/full-go-data-providers-api-switch`  
> Review 范围：Go 行情服务、Go 分析 API、Dockerfile、Compose、环境变量与首次部署流程。

## 1. Review 结论

**静态代码与 Go 自动化测试通过后，建议按本文的预检脚本进行首次部署；不能仅凭 CI 承诺真实服务器必然一次成功。** 真实部署还依赖当前服务器上的 Docker 网络、数据库是否存在、账号权限、Redis 密码和行情供应商网络。

本分支包含根 Go 分析 API 和原生 Go market-data 两个部署服务。该 Compose 文件不启动 `go-core` 或 Python bridge；当前根 API 的行情与日 K 不依赖 Python 行情 fallback。

## 2. 本轮重点核查与修正

| 项目 | Review 结果 |
|---|---|
| Go 模块测试 | 根 module、`go-core`、`services/market-data` 的格式检查、测试、race 和 vet 已在既有 CI 运行 #147 通过 |
| Docker 实际构建 | 之前 CI 没有构建两个 Dockerfile。本轮已把 Compose 语法检查和两个镜像构建加入 CI，结果以本分支最新 Actions 运行记录为准 |
| Compose 配置输入 | 两个容器共用同一个 `MARKET_DATA_API_KEY`；行情服务可使用拆分后的 `MARKET_DATA_DB_*`，旧 `MARKET_DATA_MYSQL_DSN` 暂时兼容 |
| 数据库/Redis 复用 | 两个容器连接现有 `1panel-network` 内服务，不应再建第二套数据库容器 |
| 宿主机端口安全 | 分析 API 没有内置用户认证，宿主机映射已限制为 `127.0.0.1:8092`，避免直接绑定公网网卡 |
| MariaDB 凭据 | 分析 API 使用 MySQL 驱动原生 Connector，不把密码拼入 DSN 字符串；新增测试覆盖包含 `@`、`:`、`/`、`?`、`#`、`&`、`# Go 服务部署就绪代码 Review

> Review 日期：2026-10-22  
> 目标分支：`feat/full-go-data-providers-api-switch`  
> Review 范围：Go 行情服务、Go 分析 API、Dockerfile、Compose、环境变量与首次部署流程。

## 1. Review 结论

**静态代码与 Go 自动化测试通过后，建议按本文的预检脚本进行首次部署；不能仅凭 CI 承诺真实服务器必然一次成功。** 真实部署还依赖当前服务器上的 Docker 网络、数据库是否存在、账号权限、Redis 密码和行情供应商网络。

本分支包含根 Go 分析 API 和原生 Go market-data 两个部署服务。该 Compose 文件不启动 `go-core` 或 Python bridge；当前根 API 的行情与日 K 不依赖 Python 行情 fallback。

## 2. 本轮重点核查与修正

| 项目 | Review 结果 |
|---|---|
| Go 模块测试 | 根 module、`go-core`、`services/market-data` 的格式检查、测试、race 和 vet 已在既有 CI 运行 #147 通过 |
| Docker 实际构建 | 之前 CI 没有构建两个 Dockerfile。本轮已把 Compose 语法检查和两个镜像构建加入 CI，结果以本分支最新 Actions 运行记录为准 |
| Compose 配置输入 | 两个容器共用同一个 `MARKET_DATA_API_KEY`；行情服务可使用拆分后的 `MARKET_DATA_DB_*`，旧 `MARKET_DATA_MYSQL_DSN` 暂时兼容 |
| 数据库/Redis 复用 | 两个容器连接现有 `1panel-network` 内服务，不应再建第二套数据库容器 |
、`%` 的凭据 |
| 构建上下文 | 根目录与 `services/market-data` 分别有 `.dockerignore`，避免把本地 `.env` 带入 Docker 构建上下文 |
| 部署可操作性 | 新增 `scripts/deploy-go.sh`，按顺序检查配置、构建、启动并等待健康状态；失败会输出最近日志 |

## 3. 部署前必须满足的条件

1. 已存在 Docker 网络 `1panel-network`，现有 MariaDB 与 Redis 容器已加入该网络。
2. MariaDB 数据库本身必须预先存在；自动迁移只创建表，不会自动创建数据库。
3. `.env` 中的 MariaDB 数据库名、账号、密码和服务主机名都是真实值；不能直接使用 `.env.example` 中的占位值。
4. `MARKET_DATA_API_KEY` 是不少于 32 个字符的随机密钥；两个 Go 服务必须使用同一值。
5. 如果 `DB_AUTO_MIGRATE=true` 和 `MARKET_DATA_DB_AUTO_MIGRATE=true`，对应账号必须对目标数据库有建表和变更表结构所需的权限。没有权限时服务会在启动/就绪检查阶段失败。
6. `MARKET_DATA_REDIS_ADDR` 与 `REDIS_ADDR` 应使用网络内可解析的 Redis 地址；两个变量可使用相同地址及密码，也可以单独配置。
、`#` 或空格，应在 `.env` 中使用单引号包裹完整值，避免 Compose 插值或行内注释解析改变密码；不要把真实凭据提交到 Git。

## 4. 推荐的一次性部署流程

如果服务器已经有 `.env`，**不要复制模板覆盖它**。先把其中过时的 `MARKET_DATA_MYSQL_DSN` 迁移为 `MARKET_DATA_DB_HOST/PORT/USER/PASSWORD/NAME/CHARSET/TIMEZONE` 更清晰；如需先验证旧配置，且没有设置任何拆分连接字段，当前代码仍暂时兼容旧 DSN。

在仓库根目录执行：

```bash
bash ./scripts/deploy-go.sh
```

脚本会检查 Docker/Compose、根目录 `.env` 和 `1panel-network`；用静默模式检查 Compose 配置，不把解析后的密钥打印到终端；构建两个镜像、启动服务并等待容器健康；最后请求 `/healthz` 与 `/readyz`。失败时输出最近日志帮助定位连接、密钥、权限和配置问题。

## 5. 部署后验收

```bash
docker compose -f docker-compose.go.yml ps
docker compose -f docker-compose.go.yml logs --since=10m market-data stock-analysis-go
curl -fsS http://127.0.0.1:8092/healthz
curl -fsS http://127.0.0.1:8092/readyz
docker exec stock-market-data wget -qO- http://127.0.0.1:8091/health/ready
```

再用 market-data 容器中实际注入的 API Key 请求一次真实行情，确认上游网络工作正常：

```bash
docker exec stock-market-data sh -lc 'wget -qO- --header="X-API-Key: ${MARKET_DATA_API_KEY}" "http://127.0.0.1:8091/api/v1/quotes/600519.SH"'
docker exec stock-market-data sh -lc 'wget -qO- --header="X-API-Key: ${MARKET_DATA_API_KEY}" "http://127.0.0.1:8091/api/v1/bars/600519.SH/daily?limit=30&adjustment=forward_adjusted"'
```

真实行情端点还受外部供应商网络、限流和交易时段影响；健康检查成功表示服务与 DB/Redis 已就绪，不等于所有数据源当时都能返回有效行情。

## 6. 外网与 OpenResty 注意事项

- 当前分析 API 没有内置用户认证；`127.0.0.1:8092` 只限制宿主机端口绑定，不能代替反向代理鉴权。
- 如果 OpenResty 容器加入 `1panel-network`，上游使用 `http://stock-analysis-go:8092`；如果没有加入网络，应先配置网络连通性，不要把容器内的 `127.0.0.1:8092` 当作 Go API 地址。
- 对公网开放域名之前，必须通过 OpenResty 配置 HTTPS、认证和来源限制。

## 7. 尚未被自动化测试覆盖的外部条件

CI 能验证 Go 代码、Compose 语法和 Docker 镜像构建，但无法替代目标服务器上的真实 MariaDB migration、账号授权、Redis 认证、网络 DNS、供应商响应和端到端分析请求。部署脚本完成后，仍要执行本节的健康检查和真实行情 smoke test。市场广度、板块、财务、龙虎榜、新闻公告等数据域也尚未全部迁移到 Go；这不阻止两个容器启动，但可能使部分分析结果处于 `partial` 状态。
