# Go 服务部署就绪 Review

> Review 日期：2026-10-22  
> 分支：`feat/full-go-data-providers-api-switch`  
> 目标：降低首次部署的配置与构建风险；真实服务器验收仍以实际运行结果为准。

> **当前状态（2026-10-26，核验提交 `dc7b0c3`）**：部署入口已统一到根目录 `docker-compose.yml`，服务名为 `stock-analysis-data`、`stock-analysis-server`、`stock-analysis-web`。最新 Go 测试套件 [#40314002470](https://github.com/Kasign/stock_data_provider/actions/runs/40314002470)、WebUI 检查 [#40314002521](https://github.com/Kasign/stock_data_provider/actions/runs/40314002521) 和 Live provider probe [#40314002466](https://github.com/Kasign/stock_data_provider/actions/runs/40314002466) 均已通过。统一 Compose 解析、部署脚本语法检查、两个 Go 镜像构建、WebUI 单测与 Nginx 镜像构建均已在 CI 验证；目标服务器的实际部署与数据库/Redis/网络验收仍未完成。

## 1. Review 结论

在核验提交 `dc7b0c3` 上，Go 根 module、`go-core`、`services/market-data` 均通过 `gofmt`、`go test ./...`、`go test -race ./...` 和 `go vet ./...`。Go 测试套件 [#40314002470](https://github.com/Kasign/stock_data_provider/actions/runs/40314002470) 同时通过统一 Compose 配置解析、部署脚本 `bash -n`、行情 API 镜像与分析 API 镜像构建；WebUI 检查 [#40314002521](https://github.com/Kasign/stock_data_provider/actions/runs/40314002521) 通过 JavaScript 语法、单测和 Nginx 镜像构建。Live provider probe [#40314002466](https://github.com/Kasign/stock_data_provider/actions/runs/40314002466) 也通过。

这证明代码可以编译、Compose 文件可解析；**不等于目标服务器上的数据库权限、Redis 认证、Docker 网络和外部行情供应商已经通过端到端测试**。

统一 Compose 启动 `stock-analysis-data`、`stock-analysis-server` 与 `stock-analysis-web` 三项服务；它不启动 `go-core` 或 Python bridge。分析 API 的实时行情和日 K 由 Go 行情服务提供。

## 2. Review 重点

| 项目 | 结果与注意事项 |
|---|---|
| Go 代码 | 根 module、`go-core`、`services/market-data` 通过 gofmt、测试、race 和 vet |
| Docker 构建 | 两个生产 Dockerfile 均在 CI 中构建成功 |
| 数据库配置 | 分析 API 使用 `DB_*`；行情服务使用 `MARKET_DATA_DB_*`，可暂时兼容旧 `MARKET_DATA_MYSQL_DSN` |
| Redis 配置 | 分析 API 使用 `REDIS_*`；行情服务使用 `MARKET_DATA_REDIS_*` |
| 网络 | 三个产品容器连接现有 `1panel-network`，不额外部署 MariaDB/Redis |
| 端口安全 | 分析 API 的宿主机映射绑定在 `127.0.0.1:8092`；对公网开放前仍需 OpenResty 鉴权和来源限制 |
| 部署脚本 | `scripts/deploy-go.sh` 会预检旧 Go 容器与同名旧 WebUI 容器、检查 Compose、构建镜像、启动容器、等待健康检查并在失败时打印日志；容器来源不明确时在构建前停止，不自动删除 |

## 3. 部署前必须确认

1. Docker 网络 `1panel-network` 已存在，MariaDB 和 Redis 容器已接入该网络。
2. 目标 MariaDB 数据库（例如 `stock_analysis_server`）已经创建。自动迁移只创建本项目的数据表，不会创建数据库本身。
3. DB 用户、密码、数据库名和服务地址正确。开启 `DB_AUTO_MIGRATE=true` 或 `MARKET_DATA_DB_AUTO_MIGRATE=true` 时，相应账号要有建表权限；已有表结构需要升级时也要有相应变更权限。
4. 两个 Go 服务必须使用相同的 `MARKET_DATA_API_KEY`，且密钥至少 32 个字符、不能是占位值。
5. Redis 地址和密码可以连接现有实例；如果 Redis 使用 ACL 用户名，还应设置行情服务的 `MARKET_DATA_REDIS_USERNAME`。
6. 如果使用旧 `MARKET_DATA_MYSQL_DSN`，仅在所有 `MARKET_DATA_DB_HOST/PORT/USER/PASSWORD/NAME/CHARSET/TIMEZONE` 拆分字段都未设置时才会走兼容路径。新配置建议使用拆分字段，避免 DSN 中的特殊字符问题。

### 你当前配置里必须先处理的项目

你前面贴出的 `MARKET_DATA_API_KEY=change_me` 若是服务器上的实际值，就会导致两个 Go 服务启动失败。运行部署脚本前，在服务器执行：

```bash
openssl rand -hex 32
```

把生成的值填入根目录 `.env` 的 `MARKET_DATA_API_KEY`。不要把密钥贴到聊天或提交到 Git。已有真实 `.env` 时不要用模板覆盖。

若数据库密码包含 `$` 或 `#` 等特殊字符，建议在 `.env` 里用单引号包裹完整值，避免 Compose 插值或注释解析改变密码。

## 4. 推荐的一次性部署流程

在仓库根目录执行：

```bash
bash ./scripts/deploy-go.sh
```

脚本会检查 Docker / Compose、根目录 `.env` 和 `1panel-network`，以静默模式检查 Compose 配置，构建三个镜像、启动服务并等待三个容器健康，最后检查 `/healthz` 和 `/readyz`。失败时会打印服务日志。

部署后可再次检查：

```bash
docker compose -f docker-compose.yml ps
docker compose -f docker-compose.yml logs --since=10m stock-analysis-data stock-analysis-server stock-analysis-web
curl -fsS http://127.0.0.1:8092/healthz
curl -fsS http://127.0.0.1:8092/readyz
docker exec stock-analysis-data wget -qO- http://127.0.0.1:8091/health/ready
```

## 5. 验证真实行情链路

健康检查成功只能证明服务、MariaDB 和 Redis 就绪；它不证明供应商当时可以返回有效行情。继续从 market-data 容器中调用真实行情与日 K：

```bash
docker exec stock-analysis-data sh -lc 'wget -qO- --header="X-API-Key: ${MARKET_DATA_API_KEY}" "http://127.0.0.1:8091/api/v1/quotes/600519.SH"'
docker exec stock-analysis-data sh -lc 'wget -qO- --header="X-API-Key: ${MARKET_DATA_API_KEY}" "http://127.0.0.1:8091/api/v1/bars/600519.SH/daily?limit=30&adjustment=forward_adjusted"'
```

如果真实行情请求失败，检查供应商网络、限流、返回质量和服务日志；不要因为 `/readyz` 通过就认定行情源一定可用。部分数据域仍未迁移至 Go，分析结果出现 `partial` 也不一定代表部署失败。

## 6. OpenResty 与对外访问

- 宿主机进程可以通过 `http://127.0.0.1:8092` 访问分析 API。
- 如果 OpenResty 是连接到 `1panel-network` 的容器，上游地址使用 `http://stock-analysis-server:8092`。
- 当前分析 API 没有内置用户认证。对公网开放前，必须在 OpenResty 设置 HTTPS、鉴权和来源限制；不要直接把 API 端口暴露到公网。

## 7. 验收边界

本次 CI 已覆盖 Go 测试、竞态检测、静态检查、统一 Compose 解析、部署脚本语法与两个 Go 镜像构建，并通过了独立 Live provider probe；它不代表目标服务器已经部署成功，也无法验证目标环境的真实数据库是否存在、账号授权、Redis 认证、`1panel-network` DNS、OpenResty 路由与安全策略。完成目标服务器上的部署脚本、容器健康检查及真实行情 smoke test 后，才算完成首次部署验收。


## 8. 旧 WebUI 容器名冲突预检

旧版 `web/docker-compose.yml` 与统一后的根目录 `docker-compose.yml` 都使用固定容器名 `stock-analysis-web`。两者可能属于不同 Compose 项目；仅依赖 `docker compose up --remove-orphans` 不能保证清除另一个项目创建的同名容器。

部署脚本现在会在配置解析、镜像构建之前读取已有 `stock-analysis-web` 的 `com.docker.compose.project.config_files` 标签。只有该标签包含当前仓库根目录的 `docker-compose.yml` 时才继续；标签缺失或指向其他 Compose 文件时会明确停止并提示人工检查迁移。脚本不会自动移除不明来源的容器，以免破坏用户仍在使用的旧服务。按 README 迁移步骤移除应用容器不会删除外部 MariaDB/Redis 数据。

2026-10-26 的 CI 在提交 `dc7b0c3` 上通过了 Go 测试套件、WebUI 检查和 Live provider probe。CI 验证的是构建与自动化检查，不替代目标服务器上的首次部署验收。