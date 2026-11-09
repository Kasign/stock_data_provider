# Market Data 模块化单体迁移计划与进度

- 维护分支：`feat/full-go-data-providers-api-switch`
- 迁移决策：采用 Root Server 同进程模块化单体
- 方案依据：`doc/整体任务规划/04-架构方案/Market Data并入Root Server模块化单体调整方案.md`
- 总体规划：`doc/整体任务规划/01-项目总览/整体任务规划总览.md`
- 跨模块准入：`doc/整体任务规划/01-项目总览/跨模块接入执行说明.md`、`doc/整体任务规划/01-项目总览/跨模块接入准入清单.md`
- 本记录建立时的代码基线：`d8595f408337f62be3697aac661ba1570fdd3667`
- 最近一次 M0 审计基线：`9687337743ff5ad8b5b81209831d43689693ae47`
- 当前开发分支 HEAD（合并 integration 后、本轮最新代码提交）：`be892d89f4247670efd1c64774d4866e03ccc454`（合并 integration；本轮后续代码以新提交为准）
- 当前实施状态：`M1_completed / M2_completed / M3_in_progress / M4_in_progress`；M3 的 Data 侧进程内路由准备已完成，Root Server 实际装配仍待 Server 组合根完成；M4 的 borrowed DB 与 go-redis Cache Adapter 已实现，Root 侧资源注入/配置一致性及 Provider 网络出口适配仍未完成。

> 本文记录 Data 从独立 Go Module/服务迁入 Root Server 根 Go Module、同进程运行的实施计划和进度。若本文与上述架构决策文档冲突，以架构决策文档为准；公共平台及 DATA 契约以冻结契约文档为准。本文不重新定义契约。

## 当前 Review 与执行收敛决议（2026-11-09）

**Review 结论：原计划方向没有问题，执行管理方式需要修正。** 方案 B（Data 与 Root Server 同 Go Module、同进程；Data 保留领域所有权；Server 作为唯一组合根）与项目总览一致，不需要重新讨论架构，也不需要再增加一份方案文档。

此前反复推进的根因是：M0 没有把“已审计”“已自动化”“已通过 CI”“需要外部生产证据”分开；CT-05 每轮都扩大参数测试范围，却没有固定关闭条件。现按上面的 M0-A～M0-D 收敛，历史审计记录只作为证据，不再生成新的审计轮次。

### 当前代码与 CI 基线

- 当前开发分支 HEAD：`b97181d1d03b9e2898ebb2acad29b1d084bb5e37`；已通过 PR #46 合入 integration，Merge Commit：`354c3e83db21ae1bca48a19caa9fea11d8cc15fd`。
- 当前 integration 集成验证使用 PR workflow Run `42557067351`（head `b97181d1`，目标分支 integration）。
- Data Module Job `128965072448`：gofmt、go test、race、vet 全部 success。
- Market Data MariaDB Migration/PIT Job `128965072409`：success；News MariaDB integration Job：success。
- Compose 校验与 Market Data / Analysis API 两个镜像构建 Job `128965072015`：success。
- go-core Job：gofmt、test、race、vet 全部 success。
- Root `Test .` Job `128965072366`：失败于全仓 gofmt，测试/race/vet 被跳过。失败文件均属于其他模块：`internal/llm/**`、`internal/news/app/authorization_snapshot.go`、`internal/notification/**`、`internal/quant/**`、`internal/research/**`、`internal/researchflow/**`；Data 目录不在失败清单内。本分支不越界修改。
- Platform Go checks、Go-Python parity、Live Provider probe workflow success；Live Provider 外层 success 不等于每个 Provider/能力/证券/日期/复权口径均完成真实数据验收。
- WebUI checks failure 属于独立 WebUI 门禁，不计入 M0-D 的 Data 模块验收。
- **M0-D 尚未关闭：** 需其他模块完成 gofmt 后，在最新 integration HEAD 重新执行根模块 gofmt/test/race/vet；Data 自身集成 Job 已通过。

### 方案边界校准

1. MDM-001/M0 负责迁移前盘点、契约差异修复和证据冻结；M0-A/M0-B 已关闭，不再扩大审计范围。
2. MDM-002/M1 已启动，处理根与 Data Module 的 Go 版本、依赖、go.mod/go.sum 和目录迁移；不等待其他模块全仓门禁全部通过。
3. CT-01～CT-06 是 M0 的验收切片，不是与 M0 并列的第二套主计划。
4. Data 领域数据质量/Provider 阶段 A～I 与 MDM 结构迁移是两条独立轨道；不得用 DailyBar 真实数据验收代替 MDM，也不得用 Module CI 代替真实 Provider 验收。
5. DATA-CTR、PLAT-CTR、GOV-CTR 的语义和冻结状态以项目契约登记簿及生产方文档为准；本迁移文档只记录迁移适配，不复制字段定义。

### 下一批唯一执行顺序

1. 完成 M0-A：对现有 35 Operation 参数矩阵做最后一次差异核对，只修真实差异，输出差异归零表。
2. 完成 M0-B：把 Provider 请求安全审计收敛成调用点矩阵；已有测试只引用，不重复实现。只补影响迁移安全且缺少证据的测试。
3. 取得脱敏生产 DDL，完成 M0-C；这是外部输入，不再通过反复读取仓库替代。
4. M0-A～C 完成后，按用户既定流程将当前分支合入 integration 并在 integration 验收，关闭 M0。
5. M0 关闭后才开始 M1；M1 只处理 Module 归并，不顺带重写 API、Provider 或质量规则。

**停止条件：** M0-A/M0-B 已关闭，不再为了追求“测试数量更多”继续扩张；除非出现新的 Handler 行为或真实故障证据，否则不重新打开已关闭项。M0-C 生产 DDL 是生产切换证据，不阻断代码迁移；M0-D 全仓门禁改为持续集成项，只在出现 Data 直接依赖的集成故障时阻断对应步骤。

## 1. 目标与边界

### 1.1 目标

将 `services/market-data` 从独立 Go Module 和独立默认部署单元迁入根 Go Module，由 Root Server 作为唯一组合根，在同一进程中装配 Analysis 与 Market Data。

迁移后：

- Market Data 保留独立领域边界、数据事实所有权、Provider、标准化、质量、PIT、Repository、表结构和 Migration 所有权。
- Server 负责进程入口、配置聚合、依赖装配、共享基础设施生命周期、路由、认证上下文和优雅关闭。
- Analysis 等消费者只依赖 Data 的公开 Application Port，不依赖 Provider、Repository、内部 Model 或 SQL。
- Data HTTP Adapter 可保留，供兼容 API 或未来远程部署使用；Server 内部默认走进程内 Port，不通过 HTTP 自调用。
- WebUI 继续使用兼容的 `/market-api/` 路径，但由 Root Server 提供路由。
- 不因迁移改变数据语义、API DTO/Schema、数据库结构或历史数据。

### 1.2 非目标

本次迁移不包含：

- 重写 Provider、行情标准化、单位/精度、复权、交易日历、质量和 PIT 规则。
- 数据库表重命名、历史数据搬迁、数据清洗或重新计算。
- 将 Data Repository、SQL 或领域规则移交给 Server/Platform。
- 把 News、Quant、Researchflow 等其他模块一并重构。
- 删除远程 HTTP Adapter 或历史 go-migration profile（除非后续单独决策）。
- 在迁移阶段提前实施尚未冻结或仍有边界争议的公共契约改造。

### 1.3 关键约束

1. 只在当前分支实施；所有分支更新使用最新 HEAD 和 `expected_sha`。
2. 先完成迁移，再按冻结契约接入公共平台能力；不得把迁移与大规模契约重构混成一个提交批次。
3. 迁移前后保持 Provider 行为、API 语义、数据库 Schema、PIT 和质量状态一致。
4. 不允许消费者绕过 Application Port 访问 Data 内部包或数据库。
5. 不允许同一业务调用隐式地在 InProcess 与 Remote HTTP 两条路径间 fallback。
6. 共享资源必须明确 borrowed/owned 语义，禁止重复创建连接池或重复关闭共享资源。
7. Migration 由组合根显式注册和执行；不得在连接池 Open 中隐式执行。
8. 不把 CI 合约测试通过等同于真实 Provider 数据验收通过。

## 2. 目标结构

建议目标结构（具体包名可根据现有代码做最小调整，不为目录形式而机械重构）：

```text
cmd/
  server/
    main.go

internal/
  server/
    bootstrap/
    transport/
    config/

  marketdata/
    app/             # 对外 Application Port / UseCase
    domain/          # 领域规则与模型
    provider/        # Provider Adapter
    repository/      # Data Repository Adapter
    cache/           # Data Cache Adapter
    normalize/       # 标准化与质量规则
    httpapi/         # 兼容 HTTP Adapter
    config/
    bootstrap/       # 模块构造、Worker 生命周期

  platform/
    ...              # 公共基础设施
```

第一阶段允许先将现有代码整体迁入根 Module 下的临时兼容目录，保持 package 名和相对结构，优先完成 Module 归并与编译；后续再以小批次提取 Application Port 和 Bootstrap。不得将“迁移”扩大成无关的领域分层重写。

## 3. 阶段计划与验收门槛

### M0：迁移前代码审计与清单冻结

状态：`in_progress`（审计收敛阶段；不再无限扩展审计轮次）

**本阶段只关闭迁移前置事实，不实施 Module 归并、Bootstrap 重构或部署切换。** 历史 M0 第一轮至第二十九轮记录作为审计证据，不再作为新的任务队列；CT-01～CT-06 是 MDM-001 的验收切片，不是六条可以无限扩张的独立主线。

#### 已完成并有代码/文档依据的审计项

- [x] 根 Module 与 Data Module 的版本、依赖、入口、目录和构建边界已盘点；Go 版本/依赖冲突留给 M1 解决，不在 M0 提前改动。
- [x] Data package、Provider、HTTP API、Model、Repository、Cache、Config、Migration、Worker、测试与 Docker 上下文已形成目录级清单。
- [x] Root → Data 调用链、Remote Adapter、DTO 映射、认证方式、超时和 WebUI/Nginx 路由依赖已定位。
- [x] MariaDB Pool、Redis Client、Migration、Probe 持久化、Retention Worker 的创建、所有权和关闭风险已记录。
- [x] Compose、Dockerfile、CI workflow、部署脚本和历史 go-migration profile 已盘点。
- [x] CT-01 路由/认证、CT-02 Error Response、CT-03 Go Model/OpenAPI 全字段与序列化、CT-04 LimitPool 条件语义已有实现与自动化测试；不重复重做。
- [x] 35 Operation 的参数名称/位置/required 清单和类型/格式矩阵已建立；Probe Body Schema 已有独立校验。
- [x] 当前 SHA `5a25e332` 的 Data Module gofmt、go test、race、vet、MariaDB/PIT/Probe 集成及 Market Data Docker 构建通过（Push workflow 证据，非 integration 最终验收）。

#### M0 仅剩的关闭项

| 编号 | 剩余项 | 完成条件 | 当前判定 |
|---|---|---|---|
| M0-A / CT-05（CT-03 复核） | 35 Operation 参数语义最终核对 | 固定参数全集的 inventory、类型/格式、枚举、数值边界/默认值、日期与重复值语义已覆盖；CT-03 Go Model/OpenAPI 全字段、required/nullable、序列化测试复核无当前差异；SHA `77ebff65` Data gofmt/test/race/vet 全通过 | `completed` |
| M0-B | Provider 请求行为安全证据 | 已完成 HTTP/TCP 出站调用矩阵；当前配置边界与重定向风险明确登记，归入 M4 Platform HTTP Client / TCP 出站策略整改 | `completed_with_M4_followups` |
| M0-C | 生产数据库 DDL 对照 | 获取脱敏生产 Schema/DDL；逐表对照 Data Migration、索引、唯一键、字段类型/精度/NULL/默认值；CI 临时 MariaDB 不能替代生产证据 | `blocked_external_evidence` |
| M0-D / CT-06 | 集成基线验收 | PR #46 已以 Merge Commit 合入，integration HEAD=`354c3e83`；Data gofmt/test/race/vet、MariaDB/PIT、Compose/双镜像通过；Root `Test .` 被其他模块 gofmt 阻断，等待模块 Owner 修复后重跑 | `blocked_by_other_modules` |

#### CT-05 停止扩张规则

1. 参数核对以 35 Operation 为固定全集，不再增加接口范围。
2. 只检查 Handler 实际实施、且 OpenAPI Schema 能表达的限制；无运行时依据的“建议限制”不写入契约。
3. 已有参数清单、类型/格式矩阵、Probe Schema、默认值测试和边界描述测试不重复实现。
4. 对每个剩余参数只记录：Handler 校验位置、OpenAPI 当前值、差异、是否需要修复、对应测试。没有差异即关闭，不再为了增加测试数量增加测试。
5. 本轮最终输出一张差异表；完成后 CT-05 关闭。后续若 Handler 规则变化，由对应代码变更同步更新测试，不再持续做无边界的“全量审计”。


#### M0-B Provider 出站请求审计结果（代码基线：`e04c2e4a`）

审计范围：`services/market-data/internal/provider/**` 与 `internal/baostock/client.go`。本项只冻结现状和迁移要求，不在 M0 重写各 Provider 网络栈。

| Provider | 传输/方法 | 超时与响应体 | 重定向 | 认证/敏感信息 | 结论 |
|---|---|---|---|---|---|
| Eastmoney | HTTP GET/POST；统一 `getJSON` 与专项请求 | Client Timeout；成功响应使用 `readBoundedBody(maxProviderPayloadBytes)`；错误体最多读取 4 KiB | 自定义策略：最多 3 跳，仅允许 HTTPS Eastmoney 域名 | 无 Provider API Key | 已有较强重定向边界；仍缺 DNS/IP/解析后地址约束 |
| Hithink | HTTP GET | Client Timeout；成功响应 `readBoundedBody(maxProviderPayloadBytes)`；错误体 4 KiB | 默认 Go 自动跟随 | `X-api-key` Header | 需在 M4 接入受控 HTTP Client，禁止任意跨域/非 HTTPS 跳转 |
| MiaoXiang | HTTP POST | Client Timeout；成功响应上限 2 MiB；错误体 4 KiB | 禁止跟随 | `apikey` Header；请求 endpoint 校验绝对 HTTP(S)、无 userinfo/query/fragment，但允许 HTTP | 响应受限；M4 收紧 HTTPS 和 Host/IP 策略 |
| NetEase | HTTP GET | Client Timeout；成功响应走有界读取；错误体 4 KiB | 禁止跟随 | 无 API 凭据 | 响应与跳转边界已有保护；仍缺 DNS/IP 策略 |
| Sina | HTTP GET | Client Timeout；成功响应上限 1 MiB；错误体 4 KiB | 禁止跟随 | 无 API 凭据 | 响应与跳转边界已有保护；仍缺 DNS/IP 策略 |
| Tencent | HTTP GET | Client Timeout；成功响应 `readBoundedBody(maxProviderPayloadBytes)`；错误体 4 KiB | 禁止跟随 | 无 API 凭据 | 响应与跳转边界已有保护；仍缺 DNS/IP 策略 |
| TickFlow | HTTP GET/POST | Client Timeout；成功响应 `readBoundedBody(maxProviderPayloadBytes)`；错误体 2 KiB | 默认 Go 自动跟随 | `x-api-key` Header | 需在 M4 接入受控 HTTP Client，重点阻止认证请求跨域重定向 |
| Tushare | HTTP POST | Client Timeout；成功响应有界读取；错误体 4 KiB | 默认 Go 自动跟随 | token 放在 JSON 请求体 | 需在 M4 禁止非预期重定向；请求体携带凭据，重定向必须拒绝而非依赖 Header 自动剥离 |
| Xueqiu | HTTP GET | Client Timeout；成功响应有界读取；错误体 4 KiB | 默认 Go 自动跟随 | `Cookie` Header | 需在 M4 接入受控 HTTP Client，禁止携带 Cookie 跨域跳转 |
| Yahoo | HTTP GET | Client Timeout；成功响应有界读取；错误体 4 KiB | 禁止跟随 | 无 API 凭据 | 响应与跳转边界已有保护；仍缺 DNS/IP 策略 |
| BaoStock | 原生 TCP | ConnectTimeout、连接读写超时及单连接门控；非 HTTP Body | 不适用 | 登录凭据通过 BaoStock TCP 协议发送 | 不接入 Platform HTTP Client；M4 单独明确 TCP Host/Port 出站白名单 |
| Pytdx | 原生 TCP | Dial 最长 1.5 秒/次，最多轮询 6 个候选；Client 设置整体 timeout | 不适用 | 无 HTTP 凭据 | 不接入 Platform HTTP Client；M4 单独明确 TCP Host/Port 出站白名单 |

**审计结论与边界：**

1. Provider endpoint 来自启动配置/构造参数，不来自当前 Data HTTP API 的请求参数；本轮未发现调用方可通过业务 API 直接指定任意上游 URL 的入口。此结论不等于 endpoint 配置天然可信，配置写入权限仍属于部署/运维信任边界。
2. HTTP Provider 均设置 Client Timeout；审计到的成功响应路径均使用有界读取，错误响应体按 2/4 KiB 等小上限读取。Eastmoney、Hithink、MiaoXiang、NetEase、Sina、Tencent、TickFlow、Tushare、Xueqiu、Yahoo 均已核对。
3. Hithink、TickFlow、Tushare、Xueqiu 使用 Go 默认重定向策略；其中 Hithink/TickFlow/Xueqiu 使用 Header 凭据，Tushare 将 token 放在 POST JSON Body。迁移后必须由受控 HTTP Client 拒绝非预期重定向，不能只依赖 Go 默认敏感 Header 剥离。
4. Eastmoney 已限制最多 3 跳且只允许 HTTPS Eastmoney 域名；MiaoXiang、NetEase、Sina、Tencent、Yahoo 禁止自动跟随重定向。
5. 当前 Provider 自建 `http.Client`，未统一执行 Platform HTTP Client 的 DNS 解析后 IP 检查、私网/保留地址拒绝和 DNS rebinding 防护。该项属于已确认的 M4 网络出口整改，不伪装成已完成安全加固。
6. BaoStock 与 Pytdx 是 TCP，不应为了复用 HTTP Client 而包装成 HTTP；M4 必须单列 TCP 目标地址白名单、连接超时和部署网络出口限制。

**M0-B 判定：** 出站调用行为审计已完成；安全缺口已明确到 Provider、敏感凭据类型和 M4 目标，不再扩大 M0 审计范围。该状态只代表“审计交付完成”，不代表 DNS/IP SSRF 防护已落地。M4 未完成前，不得把 Provider 网络出口安全整改标记为完成。

#### M0 退出门槛（已调整）

- M0-A 请求参数契约核对已完成；M0-B Provider 出站审计已完成，后续 HTTP/TCP 安全整改进入对应迁移阶段。
- M0-C 生产 DDL 是生产切换前置证据，不阻断源码迁移、Module 归并或本地/CI 集成验证；未取得 DDL 前不得宣称生产 Schema 验收通过。
- M0-D 全仓集成门禁持续运行。其他模块的 gofmt/test 失败不阻断 Data 独立迁移；若暴露 Root Server 启动、依赖接口或 Data 路由的直接冲突，则只阻断受影响的集成步骤。
- Data 迁移对象已有源路径、目标路径、Owner、测试和回滚映射。
- **M1 已准入并启动**，不再以 M0-C/M0-D 全部关闭作为前置条件。M1 期间解决 Go 版本和依赖冲突，不在 M0 先改 go.mod/go.sum。

**当前明确不做：** 不新增 API/DTO/数据库字段；不扩展 OpenAPI 测试到没有 Handler 依据的规则；不修改 Provider 业务算法；不接入 Platform HTTP Client；不搬迁目录；不修改 Root Server、WebUI 或其他模块。

### 2026-11-09：M1-1 运行入口与资源所有权盘点（代码基线 `9687337743ff5ad8b5b81209831d43689693ae47`）

本轮仅核对代码和部署事实；未修改运行时代码、数据库、配置或其他模块。

| 迁移对象 | 当前实现证据 | M1/M2 后续处理 |
|---|---|---|
| Data 进程入口 | `services/market-data/cmd/market-data/main.go` 独立加载配置、构造 Provider/Service/HTTP Handler、启动 `:8091` HTTP Server | M1 先归并 Module 并保留旧命令可构建；后续抽取 Bootstrap，由 Root 统一调用 |
| Root 进程入口 | `cmd/analysis-api/main.go` 是当前根 Module 入口，先加载 dotenv，再初始化 MariaDB/Redis、Market Data Remote Client、Analysis Runner/Task Queue 和 HTTP API | 由该入口逐步承担 Data 装配；本轮不另造第二个 Server 入口 |
| 调用链 | Root 的 `internal/adapters/marketdata.Client` 通过 HTTP BaseURL + `X-API-Key` 调用独立 Data 服务；Compose 依赖 Data healthcheck | 后续增加 InProcess Adapter，完成等价性验证后切换默认调用；Remote Adapter 保留回滚 |
| Data 配置 | `services/market-data/internal/config.Load` 读取 MARKET_DATA_* 环境变量及 `configs/providers.json`，包含 Provider URL/Token、Data API Key、DB DSN、Redis 地址、自动迁移和 Provider 优先级 | 配置解析归 Data；Root 聚合读取并注入 Secret。迁移期间保持现有环境变量兼容 |
| Provider 装配 | Data 独立入口创建 11 类 Provider、按配置校验 capability，再注入 DataService；BaoStock Client 单独 Close | 抽取为 Data Bootstrap 构造函数；HTTP Provider 接入 Platform HTTP Client 属后续网络安全适配，不与 Module 搬迁混做 |
| MariaDB | Data `repository.OpenMySQL` 内部 `sql.Open`、独立连接池、Ping、Close；`Store.Migrate` 管理 Data 表 | 需要增加借用 `*sql.DB` 的构造路径，Data 保留 Repository/Schema/Migration 所有权，Root 负责共享 Pool 生命周期 |
| Redis | Data Cache 使用自定义 RESP Client，每次命令单独 Dial；支持 AUTH/SELECT/TTL；没有共享连接池和统一 Close | 保持 Cache 接口和 key/TTL 语义，增加基于 Root `*redis.Client` 的 Adapter；不再并行保留两套生产连接池 |
| Worker | Retention Worker 在 Data main 内启动 goroutine，使用进程 Context 退出；当前无显式 Start/Stop/Wait 对象 | 提取可控生命周期组件，Root 统一 Start/Drain/Stop，增加启动失败和关闭测试 |
| HTTP | Data Handler 自己注册健康检查和 35 Operation 路由，内部包含 API Key 认证、Request ID、访问日志和超时包装 | Data 保留 Handler Adapter；Root 挂载兼容路由并统一入口 Middleware，避免重复认证/Request ID |
| 部署 | Compose 当前运行独立 `stock-analysis-data` 和 `stock-analysis-server` 两个容器；Data 独立 Dockerfile；Root 镜像仅构建 Analysis API | M1 不切部署；M5 合并为 Root Server 镜像并保留旧 Data profile 回滚 |
| Go Module | Root Go 1.22；Data Go 1.23.0。Root mysql driver 1.9.3，Data 1.8.1；x/net、x/text 版本也有差异。Data imports 使用 `github.com/Kasign/stock_data_provider/services/market-data/internal/...`，Root 使用 `example.com/stock-analysis-flow/internal/...` | M1 需统一 Go 版本/依赖、重写 Data 内部 import 路径、合并 go.mod/go.sum；不能只删除嵌套 go.mod |

#### 迁移顺序与边界确认

1. **先完成 M1 Module 归并**：Data 源码迁入根 Module 的 `internal/marketdata/**`，保留现有 package 边界和 Data HTTP API；先解决 Go 版本、依赖版本、内部 import 路径和根 CI 构建。
2. **再做 M2 Bootstrap/Application Port**：从 Data main 抽离配置、Provider 装配、Service、Handler、Worker 构造；提供显式构造与关闭接口。
3. **M3 Root 装配**：Root 注入共享 DB/Redis、Platform HTTP Client、Secret 和请求上下文；Analysis 从 Remote Adapter 切换 InProcess Adapter。
4. **M4 共享资源和网络安全适配**：DB borrowed/owned、Redis Adapter、Data Migration 显式执行、HTTP Provider 受控出口、TCP Provider 出站策略分开验收。
5. **M5 部署切换**：最后才合并容器与 Nginx 路由；生产 DDL 对照和凭据轮换在生产切换前完成。

#### 本轮发现的直接风险

- 不能把 Root 的 `mysqlredis.Store.Open` 直接复用为 Data Store：当前 Root Open 会自行执行 Analysis Migration 并校验 Analysis Schema，且同时拥有 DB/Redis；Data Repository 当前没有注入已有 `*sql.DB` 的公开构造入口。必须拆分资源打开与模块 Migration 的职责。
- Data Redis 当前每次操作新建 TCP 连接，Root 使用 go-redis 连接池；切换时必须以现有 Cache 接口测试锁定 GET/SET、AUTH、DB index、TTL 和错误语义。
- Root 当前 HTTP Adapter 使用自建 `http.Client`；Platform HTTP Client 有固定 DestinationPolicy/BoundDoer 与有界响应读取能力。Provider 侧迁移必须逐 Provider 绑定 Host/Port 策略，不能简单替换 Doer 后丢失请求差异。
- Data 的 Provider 配置文件默认相对路径为 `configs/providers.json`；迁入根 Module 后工作目录变化会导致配置文件找不到。必须明确 Root 的配置路径解析和容器挂载策略。
- 当前两个 Go Module 的 module path 不同，且 Go/依赖版本不一致；先进行依赖图冲突处理，再批量改 import 和搬目录，不能在同一提交里顺带重构领域逻辑。

#### M1-1 验收

- [x] 两个进程入口、调用链、配置源、Provider 装配、DB/Redis 所有权、Worker、HTTP 路由和部署边界已逐项定位。
- [x] 已识别 Module 归并必须处理的 module path、Go 版本和关键依赖冲突。
- [x] 已识别 Data DB/Redis 注入、配置相对路径和 Platform HTTP Client 的实际适配点。
- [ ] M1-2：依赖冲突收敛与目录归并实施（下一任务）。

### M1：Go Module 归并

状态：`in_progress`

工作项：

- [ ] 将 Data 源码和配套测试迁入根 Module 的 Market Data 领域目录。
- [ ] 统一 Go 版本和依赖版本；以依赖兼容和 CI 结果确定最终版本，不盲目降级。
- [ ] 合并根 `go.mod/go.sum`，清理嵌套 Module 依赖声明。
- [ ] 调整 Docker Builder、CI、开发脚本和相关文档的 Module 路径。
- [ ] 暂时保留 Data 原有 HTTP API 和行为；不在此阶段改变认证与业务语义。
- [ ] 保留独立 Data 命令的兼容构建能力，直到 M2/M3 完成。

验收：

- [ ] 根 Module 可构建全部 Market Data 包和测试。
- [ ] 不再依赖嵌套 Data Module 才能编译。
- [ ] Provider、标准化、质量、PIT、Repository 测试通过。
- [ ] OpenAPI/HTTP Contract 测试通过。
- [ ] 迁移前后关键 DTO 和数据库 Schema 无非预期差异。

### M2：Data Application Port 与 Bootstrap 提取

状态：`in_progress`

工作项：

- [ ] 从 Data main 中提取模块构造装配，main 不再独占 Provider/Repository/Worker 装配逻辑。
- [ ] 提供强类型 Application Port，供 Server/Analysis 进程内调用。
- [ ] HTTP Handler 只依赖 Application Port，不将 HTTP DTO 作为领域共享类型。
- [ ] 明确模块返回的 Application、HTTP Handler、Diagnostics、Migration Runner 和生命周期句柄。
- [ ] Provider、Repository、Cache、Worker 的构造与 Start/Stop 有明确所有权。
- [ ] 独立兼容入口（如保留）与 Root Server 使用同一套 Module Bootstrap。
- [ ] 初始化失败时逆序释放已创建资源；Start/Stop 支持取消、超时和重复调用保护。

验收：

- [ ] 不启动 Data HTTP Server 也能通过 Application Port 完成核心用例。
- [ ] HTTP Adapter 与 InProcess Adapter 使用同一个 Application 实例。
- [ ] Application DTO 保留 nullable、精度、单位、来源、时间、复权、质量和 PIT 信息。
- [ ] 生命周期和初始化失败路径有自动化测试。

> **执行依赖修正（2026-11-09）：** M3 的 Root Server 装配不能先通过独立 `OpenMySQL` 为 Data 创建第二套 Pool。先完成 M4 的 MariaDB borrowed Pool 最小切片（Data Repository 接收 Root 的 `*sql.DB`，关闭 Data Store 不关闭 Root Pool），再实施 M3 路由与 InProcess 切换；Redis 适配和完整生命周期收口仍留在 M4。此调整只前移 M3 的必要资源前置条件，不提前合并 Platform 代码。

### M3：Root Server 同进程装配

状态：`in_progress`

工作项：

- [ ] Root Server Bootstrap 创建并持有共享基础设施。
- [ ] 构造 Market Data Module 并注入公开 Application Port。
- [ ] 将 Analysis 当前 Remote HTTP Client 替换为 InProcess Adapter。
- [ ] Data HTTP Handler 挂入 Root Server Router，兼容既有 `/market-api/` 路径。
- [ ] Root Server 统一入口 Request Context、认证和路由中间件。
- [ ] Data 领域授权仍由 Data 按契约执行；不因同进程取消 Scope、purpose 或审计。
- [ ] 禁止同一调用路径隐式 fallback 到 Remote HTTP。
- [ ] 明确 readiness 对 Data 模块初始化失败的反映规则。

验收：

- [ ] Analysis 只依赖 Data Application Port。
- [ ] 同进程调用结果与迁移前 Remote Adapter 的业务语义一致。
- [ ] Request Context 可贯穿入口、Application、Repository 和诊断记录。
- [ ] 外部 HTTP 路由仍执行正确的入口认证/授权。
- [ ] 不存在 Data 对 Server 的反向依赖。

### M4：共享资源与生命周期统一

状态：`pending`

工作项：

- [ ] MariaDB：Root Server 创建共享 Pool，Data Repository 明确接收 borrowed `*sql.DB`；Data 仍拥有 SQL 和 Repository。
- [ ] Repository 支持 borrowed/owned 资源语义；只有独立兼容入口关闭自有 Pool。
- [ ] Migration：Root Server 显式注册各模块 Runner，按 namespace 和依赖顺序执行；禁止 Store.Open 隐式迁移。
- [ ] Redis：Data Cache 适配共享 go-redis Client；如 Redis DB/配置不同，由组合根创建独立 Client 并明确拥有者。
- [ ] 保留 Data Cache key namespace、TTL、禁用缓存和降级语义。
- [ ] Provider HTTP：使用冻结的 Platform 受控 HTTP Client；若契约接入尚未具备条件，不自行复制安全实现，记录阻断并等待准入。
- [ ] Config：区分领域配置与远程 HTTP Adapter 认证配置；进程内模式不要求 Data API Key。
- [ ] Worker：Data 拥有执行规则，Root Server 统一 Start/Stop 和 drain。
- [ ] Logger/Diagnostics/Request Context 由组合根装配并按契约注入。

验收：

- [ ] 每个 Pool/Client/Worker 只有一个明确 owner。
- [ ] Migration 仅由组合根执行一次，失败阻止 readiness。
- [ ] Data Repository 不会关闭 borrowed Pool。
- [ ] Redis 共享或独立 Client 的配置语义可验证。
- [ ] Shutdown 顺序、超时和资源释放有集成测试。

### M5：部署与 WebUI 路由切换

状态：`pending`

工作项：

- [ ] Compose 默认只部署 Root Server 与 WebUI，不再依赖独立 Data 容器。
- [ ] Nginx 的 `/market-api/` 与 `/analysis-api/` 代理到同一 Root Server。
- [ ] WebUI 删除浏览器持有的 Data API Key、localStorage 持久化和请求头注入。
- [ ] 清理旧凭据设置；对曾暴露在浏览器中的有效 Key 制定轮换步骤。
- [ ] Provider Token 仍只由 Data Config/Provider Adapter 持有，不进入 WebUI。
- [ ] Data 配置、Worker、探测和 readiness 迁入 Root Server 部署配置。
- [ ] 保留远程 Data Adapter/历史 profile 作为显式兼容或回滚路径，直到部署验收通过。

验收：

- [ ] 全新部署不需要独立 Data 容器。
- [ ] WebUI 既有行情功能通过兼容路由正常工作。
- [ ] 浏览器存储、请求和构建产物不再包含 Data API Key。
- [ ] Server readiness、日志、诊断和优雅关闭覆盖 Market Data。
- [ ] 部署回滚步骤可执行且不破坏数据库数据。

### M6：迁移验收与稳定性观察

状态：`pending`

工作项：

- [ ] 对照迁移前基线检查 API、DTO、Provider、质量、PIT 和数据库行为。
- [ ] 验证 MariaDB/Redis 断连、恢复、超时与关闭。
- [ ] 验证 Worker 启停、取消、重复启动保护和 drain。
- [ ] 验证 Provider 超时、限流、不可用和降级不导致 Root Server 无故退出。
- [ ] 验证 Request ID、Fault、日志和诊断关联。
- [ ] 验证旧 Data HTTP Adapter 的显式兼容路径（若保留）。
- [ ] 检查重复 Pool、Provider、Worker、Migration 和双路调用。
- [ ] 形成迁移验收结论和剩余风险清单。

验收：

- [ ] 当前 HEAD 的根模块测试、race、vet、集成测试、WebUI 检查和镜像构建通过。
- [ ] 关键真实 Provider 验收单独记录；未执行/失败/跳过均不得标记通过。
- [ ] 数据库 Schema 和历史数据无非预期变化。
- [ ] 迁移回滚路径经过验证。

## 4. 迁移完成后的公共平台契约接入

迁移完成后，按冻结契约及准入清单实施，不在本迁移文档重新定义字段或语义。建议接入顺序如下，最终以契约依赖 DAG 和准入状态为准：

1. PLAT-CTR-005：受控 HTTP Client，覆盖 Provider 出站网络安全边界。
2. PLAT-CTR-006：Storage/连接池/Migration 生命周期接入。
3. PLAT-CTR-007：Config/Secret 统一装配与启动校验。
4. PLAT-CTR-002：Request Context、Request ID 和后台任务关联。
5. PLAT-CTR-001：Fault 统一映射与安全错误输出。
6. PLAT-CTR-003：Root 入口认证、Principal/Scope 注入和路由授权。
7. PLAT-CTR-004：将既有 Data Probe/Reporter/Freshness/Coverage 等诊断通过 Adapter 接入平台 Runner，不重建 Data 领域诊断。
8. DATA-CTR：按契约登记簿及准入清单逐项完成 DTO、单位精度、质量、PIT、认证和授权接入。

若某项契约在迁移期间已冻结且属于迁移正确性前置条件，可在对应阶段做最小必要适配；必须在进度记录中说明原因、范围和契约版本，不得借机扩展。

## 5. 风险与回滚策略

| 风险 | 控制措施 | 回滚原则 |
|---|---|---|
| Go 版本/依赖冲突 | 先做依赖图审计，分批调整，保留锁定版本记录 | 回退 Module 归并提交，不改业务数据 |
| 内部包越界 | 只暴露 Application Port，静态检查依赖方向 | 恢复 Remote Adapter 调用 |
| DTO 语义丢失 | 迁移前后 golden/contract 对照，nullable 与单位显式测试 | 回退 Adapter，不转换/重写历史数据 |
| DB Pool 双重所有权 | borrowed/owned 明确区分，Close 测试 | 恢复独立 Pool 但不同时启用两套生产写入 |
| Migration 重复执行 | namespace、版本表和显式 Runner | 先停止启动，不手工删除版本记录或表 |
| Redis 行为变化 | key、TTL、DB index、禁用语义对照 | 恢复旧 Cache Adapter |
| 认证边界变化 | Root 入口统一认证，Data Scope/审计继续校验 | 关闭新路由，不开放匿名访问 |
| 部署切换失败 | 保留显式旧 Data 服务部署配置和兼容 Adapter | 回滚 Compose/Nginx，不回滚数据库内容 |

## 6. 进度记录

### 2026-11-06：建立迁移跟踪记录

- 设计决策：已接受方案 B，同进程模块化单体。
- 代码迁移：未开始。
- 本记录创建时分支 HEAD：`d8595f408337f62be3697aac661ba1570fdd3667`。
- 当前仓库树仍同时存在根 `go.mod` 与 `services/market-data/go.mod`，并保留 `cmd/analysis-api`、`cmd/market-data` 两个入口；Data 仍位于独立 `services/market-data` 目录。
- 已确认存在 Root Server Remote Adapter：`internal/adapters/marketdata`。
- M0 仅完成文档/目录层面的初步定位，尚未完成逐文件依赖、资源生命周期和调用点审计；因此 M0 仍为 pending。
- 本次未修改业务代码、数据库、部署配置或契约。

### 进度状态表

| 阶段 | 状态 | 说明 |
|---|---|---|
| M0 迁移前审计 | in_progress（收敛） | M0-A/M0-B 已完成；M0-C 等待脱敏生产 DDL；M0-D 的 Root 全仓门禁受其他模块 gofmt 阻断，不阻断 Data 独立开发 |
| M1 Go Module 归并 | completed（代码归并） | Data 源码已位于根 Module 的 `internal/marketdata/**`；根 `go.mod` 为 Go 1.23.0；当前 Data 根模块 gofmt/test/race/vet 专项通过 |
| M2 Application Port/Bootstrap | completed（Data 侧） | M2-1～M2-6 已完成：Application Port、Bootstrap、Worker 生命周期、HTTP 用例接口拆分、来源能力快照和 Migration Runner |
| M3 Root Server 装配 | in_progress | Data 已提供 `InProcessHandler()`、Application Port 和 `MigrationRunner()`；Root Router、共享资源注入、Remote→InProcess 切换属于 Server 组合根任务，本分支不越界修改 |
| M4 共享资源统一 | in_progress | Data Repository borrowed DB、`cache.NewGoRedis`、Tushare Platform HTTPDoer 注入入口已实现；Root 侧实际资源/BoundDoer 注入、Redis 配置匹配、其余 HTTP Provider 与 TCP 出站策略仍待完成 |
| M5 部署/WebUI 切换 | pending | Root 同进程装配和共享资源验收后执行 |
| M6 迁移验收 | pending | 结构迁移、API/PIT/质量、真实 Provider 和部署验收分开形成证据 |
| 公共平台契约接入 | in_progress（按依赖） | 当前分支已有 Platform 基础代码；Data 仅适配已冻结能力，不复制 HTTP Client、Diagnostics Runner、Storage 等公共实现 |

### 变更记录模板

后续每轮更新按以下格式追加，避免只写“已完成”而没有证据：

- 日期与代码 SHA：
- 阶段/任务：
- 修改范围：
- 关键设计决定：
- 测试与 CI（必须注明对应 SHA/Run）：
- 未完成项/阻断：
- 回滚点：
- 下一步：

## 7. M0 审计证据总表（唯一有效清单）

本节替代历史 M0 第一轮至第二十九轮的逐轮流水账。历史提交和 CI 结果仍可从 Git 历史追溯，但不再作为活动任务清单；**当前只按本节矩阵和第 3 节 M0-A～M0-D 状态推进，不再追加“第 N 轮审计”。**

### 7.1 迁移对象、Owner、验证与回滚

| 当前源路径/资产 | 目标路径/处理 | Owner 边界 | 验证证据 | 回滚方式 |
|---|---|---|---|---|
| `services/market-data/internal/provider/**` | `internal/marketdata/provider/**` | Data 拥有 Provider 行为；Platform 拥有公共 HTTP 安全基础设施 | Provider 单测、Live Probe、Golden/Parity；HTTP/TCP 出站策略分开验收 | 回退 Provider 迁移提交，恢复独立 Data Module |
| `services/market-data/internal/baostock/**`、BaoStock TCP Client | `internal/marketdata/provider/baostock/**` | Data 拥有 TCP Client 和连接生命周期 | 协议、登录、超时、连接关闭测试 | 恢复原 Data Bootstrap 和 Client 生命周期 |
| `services/market-data/internal/normalize/**`、`validation/**`、`freshness/**` | `internal/marketdata/normalize/**`、`quality/**` | Data 独占领域规则；不得移交 Platform Diagnostics | 单测、字段诊断、Golden、Parity | 回退代码，不重算或修改历史数据 |
| `services/market-data/internal/model/**` | `internal/marketdata/domain/model/**`；HTTP/Application DTO 分离 | Data 定义领域模型；Server 只消费公开 Application DTO | OpenAPI/Go Model 全字段、required/nullable、JSON 序列化测试 | 恢复旧 Adapter/DTO 映射，不改数据库 |
| `services/market-data/internal/service/**` | `internal/marketdata/app/**` | Data 拥有用例和业务编排；对外只暴露 Application Port | Service、Fallback、PIT、质量测试 | Remote Adapter 继续调用旧 Data HTTP 服务 |
| `services/market-data/internal/repository/**`、Data Migrations | `internal/marketdata/repository/**`，保留 Data Migration Runner | Data 拥有 Repository/SQL/Schema；Root 负责显式编排 Migration | MariaDB Migration、PIT、重复执行和 borrowed DB Close 测试；生产 DDL 对照 | 停止新 Bootstrap，恢复旧 Data 服务；不回滚数据库数据 |
| `services/market-data/internal/cache/**` | `internal/marketdata/cache/**`，接入共享或独立 Redis Adapter | Data 定义 key/TTL/降级；Root 明确 Redis Client owner | Redis key、TTL、DB index、禁用和断连测试 | 恢复原 RESP Cache Adapter |
| `services/market-data/internal/config/**`、`configs/**` | `internal/marketdata/config/**`、`configs/marketdata/**` | Root 聚合配置/Secret；Data 解释领域配置 | 配置兼容、缺失 Secret、启动失败测试 | 恢复旧 env/config；轮换已暴露凭据 |
| `services/market-data/internal/httpapi/**`、`openapi.yaml` | `internal/marketdata/transport/http/**`，保留兼容 HTTP Adapter | Data 拥有 API 语义；Root 挂载路由并统一入口 Middleware | 35 Operation Contract、路由/认证/Error/JSON Golden | 将流量切回 Remote Adapter |
| `services/market-data/cmd/market-data/main.go` | 提取至 `internal/marketdata/bootstrap/**`；Root 入口负责组合 | Data Module 提供构造/生命周期；Server 唯一组合根 | Bootstrap、初始化失败逆序释放、Start/Stop/Drain 测试 | 保留旧命令入口，恢复独立部署 |
| `internal/adapters/marketdata/**` | 保留显式 Remote Adapter；新增 Data InProcess Adapter | Analysis 仅依赖 Data Application Port | Remote/InProcess 同样例 DTO 与结果一致 | 显式切回 Remote Adapter；禁止隐式双路 fallback |
| 根/服务 Dockerfile、Compose、Nginx | 单 Root Server 部署；WebUI API 前缀继续兼容 | Server/部署 Owner；Data 提供健康与诊断能力 | Compose config、镜像构建、WebUI 回归、浏览器 Secret 扫描 | 回滚 Compose/Nginx；保留旧 Data profile |
| Go Test、Parity、MariaDB/PIT workflows | Data 测试纳入 Root Module；go-core 继续独立 | 集成 Owner 管理根工作流；Data 维护本模块测试 | Root/Data/go-core、race/vet、MariaDB/PIT、Parity、Docker | 回退 workflow/Module 迁移提交 |

明确排除：`go-core/**` 不并入 Root Module；News、Quant、Research、Researchflow、LLM、Notification 不属于本分支迁移范围。

### 7.2 已冻结的迁移前事实

| 主题 | 当前事实 | 后续归属 |
|---|---|---|
| Go Module | Root 当前 Go 1.22；Data 嵌套 Module Go 1.23.0；MySQL Driver 版本存在差异 | M1 统一版本和 go.mod/go.sum |
| 入口/调用链 | Data 有独立 HTTP Server；Root 通过 `internal/adapters/marketdata` Remote Client/Collector 消费 Data | M1 保留构建；M2 提取 Bootstrap/Port；M3 切 InProcess |
| HTTP DTO/Model | HTTP Handler 当前直接使用 Data Model；迁移不能让消费者 import Data internal | M2 拆 Application DTO；M6 对照 Golden |
| MariaDB | Root 与 Data 当前各自创建 Pool；Data 仍拥有自己的 Repository 和 Migration | M4 borrowed Pool + Data Migration Runner + Root 显式编排 |
| Redis | Root 使用 go-redis Client；Data 当前为自定义 RESP Cache Client | M4 Adapter，保持 key/TTL/DB index/禁用语义 |
| Worker | Data Retention Worker 需要显式 Start/Stop/Wait/Drain 生命周期 | M2/M4 提取并由 Root 统一调度 |
| Provider 网络 | 10 个 HTTP Provider、BaoStock/Pytdx 两个 TCP Provider；详见本节前面的 M0-B 矩阵 | M4 Platform HTTP Client + TCP egress policy |
| 配置/认证 | Provider Secret 仅服务端配置；远程 Data HTTP Adapter 保留 Data Key；同进程调用使用可信 Principal/Scope | M3/M4/M5 按冻结契约切换 |
| 部署 | 当前 Compose 保留 Data 独立服务和 Root/WebUI；旧 go-migration profile 是历史兼容资产 | M5 单 Server 部署，显式保留回滚 profile |
| CI | Data 独立 Module 有 gofmt/test/race/vet、MariaDB/PIT、Compose/Docker；根模块/其他模块门禁可能独立失败 | M0-D 在 integration 做整体验收 |

### 7.3 当前 M0 证据与阻断

- **CT-01/02/04：** 路由/认证、公共 Error Envelope、LimitPool 条件语义已有对应自动化契约测试；不重复建设。
- **CT-03：** Go Model 与 OpenAPI 字段/required/nullable 对照及 JSON 序列化测试已存在。复核历史项目总览中关于 `Bundle.warnings` 必填的记录后，确认当前 OpenAPI `Bundle.required` 不包含该字段，Go `omitempty` 与当前可选契约一致；历史记录已过时，不改运行时行为。
- **CT-05：** 固定 35 Operation 参数全集；参数 inventory、类型/格式、默认值/固定边界、枚举、日期范围和重复值语义已纳入矩阵。本轮补齐 index source 和数值边界断言。最新 Data CI 结果仍以当前 SHA 对应 Run 为准。
- **M0-B：** Provider 出站行为矩阵已完成。DNS/IP SSRF 防护和默认重定向策略统一收敛到 M4；BaoStock/Pytdx TCP egress 单独治理。
- **M0-C：** 缺少脱敏生产 DDL；CI 临时 MariaDB 不替代生产 Schema 证据。用户提供 DDL 后做一次逐表对照。
- **M0-D：** 当前分支尚未完成 integration 集成验收。根模块 gofmt/其他模块失败不在 Data 分支越界修复；集成阶段只记录失败责任归属。

### 7.4 停止重复审计的规则

1. 本节迁移映射表是唯一的 M0 迁移对象清单；不再重复生成目录盘点。
2. CT-03/CT-05 以现有 Contract Test 为准，只修复可复现差异；不得为增加测试数量重复实现断言。
3. M0-B 不再重新逐 Provider 盘点；只有 Provider 新增/改变网络调用时才增量更新矩阵。
4. M0-C 只等待真实脱敏 DDL；不再用仓库内 migration 或 CI 数据库反复替代生产证据。
5. M0-D 只在 integration 执行一次集成基线验收；失败按模块归属处理，不在 Data 分支修改其他模块。
6. M0-A～D 满足关闭条件后直接进入 M1，不重新开启架构方案讨论。



### 2026-11-09：CT-03 Bundle.warnings 序列化契约修复与 M0 审计文档去重

- 项目总览中的历史复核指出 `Bundle.warnings` 在 OpenAPI 中为 required，但 Go Model JSON Tag 使用 `omitempty`；当前 Contract Test 反而将其断言为 optional，属于真实的 Schema/序列化契约不一致。
- DataService.Bundle 虽构造非 nil 空切片，但 Go `omitempty` 仍会省略长度为零的 slice；当前 OpenAPI 允许省略，因此没有契约缺陷。
- 已回退不必要的 Model/测试改动，保留原有 required/optional 和序列化断言；不改变 API 响应行为。
- M0-A 保持 CT-05 参数核对；CT-03 仅做历史差异复核，不新增阶段或测试范围。
- 将原第 7 节历史 M0 第一轮至第二十九轮流水账收敛为迁移对象映射、Owner、验证/回滚、已冻结事实和剩余阻断；历史提交仍可从 Git 追溯，不再把历史轮次作为活动任务。
- 本批仅修改 Data Model、Data HTTP Contract Test 和本迁移文档；未修改 OpenAPI、Handler、数据库、其他模块或 integration。
- 新 SHA 的 Data gofmt/test/race/vet、MariaDB/PIT 和 Docker 状态待 CI 确认；在 CI 完成前 M0-A 保持 pending_ci。



### 2026-11-09：Bundle.warnings 历史契约记录复核（纠正）

- 项目总览中的历史复核曾将 `Bundle.warnings` 记为 OpenAPI required；重新读取当前 `services/market-data/openapi.yaml` 后确认 `Bundle.required` 不含该字段。
- 当前 Go Model 使用 `omitempty`，Contract Test 将该字段断言为 optional，并验证零值序列化时省略；三者语义一致。
- 结论：历史 required 记录已过时，不存在当前契约差异；不移除 `omitempty`，不改变 API 响应行为。
- 已撤销上一提交中的临时 Model/测试改动；保留 M0 文档对该历史记录的纠正。



### 2026-11-09：M0-A 关闭与 Integration 基线风险确认

- 当前 SHA：`77ebff659b6bb8ba86a24f4924230c747aae0f1b`。
- M0-A / CT-05：35 Operation 参数清单、类型/格式、枚举、数值边界/默认值、日期范围和重复值语义核对完成；CT-03 Go Model/OpenAPI 全字段和序列化测试复核无当前差异。Data Module gofmt/test/race/vet 全部通过（Run `42556374146`），M0-A 关闭。
- Data MariaDB Migration/PIT、Compose、Market Data 与 Analysis API 镜像构建通过；Root gofmt 与 WebUI 检查仍有其他模块失败，未越界修改。
- Integration 对比：`integration` HEAD `0ad30515dc40af5e4e48d6f624880aadcea0242d`；本分支 ahead 43、behind 115，状态 `diverged`，merge-base `41e4d1d442f22e9258c576b402a123b110b886bd`。
- 不在当前分支直接制造 integration 合并提交。M0-D 等待集成窗口确认 PR 可合并性和冲突责任；不通过改写 integration 代码解决冲突。
- 当前 M0 只剩 M0-C 生产 DDL 外部证据与 M0-D integration 验收；M1 仍不准入。



### 2026-11-09：M0-D 首次集成验证结果（PR #46）

- PR：`#46`，`feat/full-go-data-providers-api-switch` → `integration`，已 Merge。
- Merge Commit：`354c3e83db21ae1bca48a19caa9fea11d8cc15fd`。本次合并仅涉及 Data 分支的 10 个文件；没有在 integration 手工修改其他模块代码。
- PR 集成 Go Test Run：`42557067351`。
- Data Module gofmt/test/race/vet、MariaDB/PIT、Compose、Market Data 镜像和 Analysis API 镜像全部通过。
- Root `Test .` 失败于全仓 gofmt，列出的未格式化文件归属 LLM、News、Notification、Quant、Research、Researchflow；因此 Root tests/race/vet 未执行。此问题不属于 Data 范围，M0-D 保持 `blocked_by_other_modules`。
- 当前 Data 分支不修复其他模块文件。待相关模块合并格式修复后，在最新 integration HEAD 重跑根模块门禁；无需重新审计 Data 参数或 Provider 矩阵。
- M0-C 仍等待脱敏生产 DDL；M0-A/M0-B 已关闭。



### 2026-11-09：M1-2 Go 工具链与依赖预收敛（进行中）

- 当前分支代码基线：`c875d5ffa00acb384f4f2b0b738b77fd10be82e6`。
- 提交 `559d1eaffa7c26da74bd34600182147e593fa803`：根 `go.mod` 升级至 Go 1.23.0，根 Docker Builder 升级至 `golang:1.23-alpine`。仓库 Go CI 已使用 Go 1.23.x，因此统一本地模块声明与生产构建工具链。
- 提交 `c875d5ffa00acb384f4f2b0b738b77fd10be82e6`：将 Data Module 依赖要求和 `go.sum` 校验和合并到根 Module；冲突版本暂选根侧较新版本：MySQL Driver 1.9.3、`x/net` 0.35.0、`x/text` 0.28.0；新增 Data 所需 `github.com/injoyai/tdx` 及其依赖。
- 依赖合并是源码搬迁的准备提交，**不代表 Data 已进入根 Module**；嵌套 `services/market-data/go.mod` 和源码目录尚未删除/迁移。
- 当前没有该 SHA 的可读取 CI 结果；不将依赖预收敛记为构建通过。源码归并后必须在根 Module 执行 gofmt、go test、race、vet、MariaDB/PIT、Provider 相关测试和镜像构建。
- 下一步：按包依赖图将 `services/market-data/internal/**` 迁入 `internal/marketdata/**`，重写内部 import 路径，迁移 Data 命令入口并同步根 CI/Docker 构建；同批删除嵌套 go.mod/go.sum，保留 Data HTTP 行为和独立回滚入口。



### 2026-11-09：当前分支 CI 归属拆分（PR #1，head `cf1d611602c1c1790a6549a1b3caba3a6e258f34`）

本次 CI 运行：Go test suite `42560316082`、Platform Go checks `42560316079`、Live provider probe `42560316101`、Go-Python parity `42560316088`、WebUI checks `42560316156`。PR merge commit 为 `50e3ac0e3f0e2bc3b72bf9a49520ceb61e1ad612`，以下结果按工作流实际运行结果记录，不将 PR merge commit 误认为 Data 分支 HEAD。

| 检查 | 结果 | Owner / 处理 |
|---|---|---|
| `Test services/market-data`：gofmt、go test、race、vet | 通过 | Data；当前无需修复 |
| Market Data MariaDB migration/PIT、DataService fallback、Probe HTTP 集成 | 通过 | Data；当前无新增失败项 |
| Go Market Data Docker image | 通过 | Data 镜像构建通过 |
| Root `Test .` gofmt | 失败：`internal/research/context/validate.go` 非法反斜杠语法；`internal/research/evidence/authorization.go:207` 含 NUL 字符 | Research；不在 Data 分支修改 |
| News MariaDB 集成 | 失败：Go 提示根模块需要 `go mod tidy` | News/根模块依赖闭包待定位；不修改 News 源码。若确认是本分支根依赖文件引入的缺项，再在本分支修复 `go.mod/go.sum` |
| WebUI checks | 失败 | WebUI；不在 Data 分支修改 |
| Platform Go checks | 通过 | Platform；无当前阻断 |
| Live provider probe、Go-Python parity | 通过 | 仅代表对应 workflow 的探测/对账通过，不等于全部 Provider 与真实数据口径均验收通过 |
| Analysis API Docker image | 运行中（记录时） | 继续等待结果；若失败按文件归属拆分 |

**本轮结论：** 当前 CI 已明确的失败项没有落在 Data 生产源码、Data 测试或 Data MariaDB/PIT 测试中，因此本分支不为通过全仓门禁而修改其他模块。M1-2 继续推进 Data Module 归并；全仓 gofmt、News 依赖闭包和 WebUI 失败分别交由对应 Owner 处理，只有确认涉及本分支拥有的根构建文件时才在此修复。



### M1-2：Data 源码归并（自动迁移批次）

- Data 生产代码、测试与 testdata 从 `services/market-data/internal/**` 迁入 `internal/marketdata/**`，保留现有 package 边界，暂不做领域包重命名。
- 独立命令从 `services/market-data/cmd/market-data` 迁入 `cmd/market-data`。
- Data 内部 import 统一切换为根 Module 路径；嵌套 `services/market-data/go.mod` 与 `go.sum` 删除，依赖由根 Module 管理。
- Provider 配置迁入 `configs/marketdata/providers.json`，默认路径同步调整。
- Data 独立 Dockerfile 改为根目录 build context；Compose 保留双容器回滚部署，Data 镜像由根 Module 构建。
- CI 增加独立 Market Data 测试门禁，MariaDB/PIT 测试与 Data 镜像构建改用根 Module 路径。
- 本批只做 Module/路径归并，不抽取 Bootstrap、不切换 Root InProcess Adapter，也不改变 Provider/领域逻辑。
- 待 CI 验证：Data gofmt、单测、race、vet、MariaDB/PIT、Docker；全仓失败按文件归属单独处理。



### 2026-11-09：M1-2 源码归并提交与回滚构建路径修正

- 自动迁移提交：`0076f99b88e8c2ae59749c1c5c341373f75e9231`。Data 源码、测试与 testdata 已迁入 `internal/marketdata/**`，独立命令迁入 `cmd/market-data/**`，Provider 配置迁入 `configs/marketdata/**`；嵌套 `services/market-data/go.mod` 与 `go.sum` 已删除。
- 根 Module 路径仍为 `example.com/stock-analysis-flow`，Go 版本保持 1.23.0；本批没有改写 Provider 领域逻辑、HTTP DTO、数据库 Schema 或历史数据。
- 迁移后 CI Run `42566947023` 已启动：Data 根 Module gofmt/test/race/vet、go-core、根模块、MariaDB/PIT、News MariaDB 和容器构建分别执行中；结果未完成前不标记验收通过。
- 复核发现旧 `docker/docker-compose.go-migration.yml` 仍使用独立 Module 的 Docker build context，且 `services/market-data/.env.example` 显式指定旧 Provider 配置路径。已同步修正这两个回滚入口，并将同样的修正规则纳入迁移脚本。
- 当前 M1-2 状态：源码迁移已提交，CI 验收待完成；后续进入 M2 Application Port/Bootstrap 提取。


### 2026-11-09：M1-2 迁移后 Data 单测失败修复

- 对应迁移验收 Run：`42567249940`，Data 根模块 Job：`128996359612`。
- 失败一：`TestLoadBuildsMySQLDSNFromSeparateServiceSettings` 仍检查 `mysql.Config.Params["charset"]`。根模块依赖合并后 MySQL Driver 从 Data 原有的 v1.8.1 统一为根模块 v1.9.3；v1.9.3 将 DSN 的 `charset` 解析进驱动内部字段，不再暴露在 `Params`。生产 DSN 仍由 `FormatDSN` 输出 charset 参数。本修复改为检查最终 DSN 字符串包含 `charset=utf8mb4`，并继续检查 ParseTime/Loc/用户/地址/库名；不改连接配置行为。
- 失败二：HTTP API Contract Tests 仍按旧目录层级读取 `../../openapi.yaml`。源码迁入 `internal/marketdata/httpapi` 后相对路径已变化，OpenAPI 文件仍由原 Data 服务目录持有。本修复将测试读取路径更新为 `../../../services/market-data/openapi.yaml`，不复制、不新增 OpenAPI 文件，也不改变 API 契约。
- 本批仅修复 Data 自有测试对迁移后目录和根模块 Driver 版本的兼容；不调整 Provider 领域逻辑、API DTO、OpenAPI 内容、数据库 Schema。
- 待新 SHA 的 Data gofmt/test/race/vet 与 MariaDB/PIT CI 复验后再关闭 M1-2。


### 2026-11-09：Data 测试路径二次修正与根模块依赖闭包定位

- 新 SHA `0ddd9e3d739728ad700191a3f69aa7467850c300` 的 Data 根模块测试已验证配置 DSN 断言修复有效。
- 仍有一项 OpenAPI Platform Fault Contract Test 使用旧相对路径（`internal/marketdata/httpapi/platform_contract_test.go`）；本批同步改为从新目录读取原 Data OpenAPI 文件。该项仍是测试路径修复，不改 OpenAPI 契约。
- 根模块 News MariaDB 测试失败原因已定位：根 `go.mod` 是多个 Module 依赖的简单合并，直接依赖仍被错误标记为 indirect/未整理；`go mod tidy -diff` 在 CI 给出可复现的依赖差异。它不是 News Repository 或 MariaDB 运行时故障。
- 本分支拥有根 Module 的依赖归并，因此在迁移工作流中加入根 `go mod tidy`，由 CI 按当前分支 push 自动整理 `go.mod/go.sum` 并提交；不修改 News 业务代码。整理范围由根 Module 全部源码/测试实际 import 决定，删除未使用依赖并补齐实际依赖校验和。
- Data 根模块测试门禁已改为在当前分支 push 时执行，避免只在手动 workflow_dispatch 时执行而日常 push 被跳过。
- 根模块全仓 gofmt 当前仍因 Research 两个文件失败，责任归 Research；本分支不修改这两个文件。


### 2026-11-09：OpenAPI 规范归位到 Data HTTP Adapter

- 架构方案 MDM-008 明确规定 M1 后 OpenAPI 规范路径为 `internal/marketdata/httpapi/openapi.yaml`；上一轮为快速修复测试路径，暂时继续读取旧 `services/market-data/openapi.yaml`，与冻结架构文档不一致。
- 本批将唯一 OpenAPI 文件迁至 `internal/marketdata/httpapi/openapi.yaml`，不复制第二份；`openapi_contract_test.go` 和 `platform_contract_test.go` 改为读取同目录文件。
- 自动迁移脚本同步增加 OpenAPI 文件迁移步骤，确保迁移重放行为与当前仓库树一致。
- 不修改 OpenAPI 内容、35 个 Operation、认证/参数/响应/错误 Schema；本次仅调整规范文件归属和测试读取路径。


### 2026-11-09：M2-1 首批 Data Application Port 定义

- M1-2 Data 专项 CI 验收：Run `42568445917`，当前 SHA `a587373105e3c4147e1cf45fe21b283c0e2e6e46`。Data gofmt/test/race/vet 全部通过；Data MariaDB Migration/PIT、日线 Fallback、PIT revision fallback、Probe HTTP E2E 全部通过；News MariaDB 集成通过。
- Root 全仓 `Test .` 仍在 gofmt 阶段被 Research 的 `internal/research/context/validate.go` 非法反斜杠和 `internal/research/evidence/authorization.go` NUL 字节阻断；本分支不修改 Research 文件。Docker 构建按用户约束未在本分支执行。
- M1-2 的 Data 专项迁移验收关闭；根模块全仓门禁仍为跨模块阻断，不将其误记为 Data 测试失败。
- M2-1 新增 `internal/marketdata/app` 首批消费者 Port：QuoteReader、DailyBarsReader、MarketDataReader。实际调用场景是现有 Analysis Collector 获取个股 Quote、个股/指数日 K；不把 35 个 HTTP Operation 全部做成一个巨大接口。
- 新增 Application DTO 与 HTTP JSON DTO 分离：QuoteSnapshot、DailyBar、DailyBarsQuery、DailyBarsSnapshot。保留 nullable 指针、Volume int64、单位、来源/版本、ProviderTime、FetchedAt/NormalizedAt/SystemObservedAt、复权、质量、fallback、RequestID 和 normalization version。
- PIT 输入字段明确命名为 `SystemObservedAtCutoff`，语义是 Data 历史修订的系统观察时点，不是交易日期，也不默认等同于 Analysis.AsOf。Server Adapter 后续必须显式定义映射；本批不修改 Server/Analysis 远程 Adapter。
- 不暴露 Provider、Repository、Cache、SQL、HTTP Handler；不新增外部 API、字段存储或诊断行为。
- 下一步 M2-2：实现 DataService 到 Application Port 的 Facade/映射，并补齐 DTO 映射、nil/精度/来源/PIT 元数据测试；M2-3 再让 HTTP Handler 依赖独立 Application 接口。


### 2026-11-09：M2-1 Port 编译验收与 M1-2 Data 专项验收确认

- 当前 SHA：`6a986d9815b6d05f2d451a8b804331a7a5ff3fa1`；M2-1 首批 Application Port 已通过 Data 根模块 gofmt、go test、race、vet（Run `42569000978`）。
- 同一 Run 中 Data MariaDB Migration/PIT、News MariaDB 集成、go-core、迁移工作流均通过；Root 全仓 `Test .` 仍仅在 gofmt 阶段被 Research 文件阻断，测试/race/vet 因前置 gofmt 失败未执行。
- M1-2 的 Data 专项验收条件已满足；根模块全仓门禁作为跨模块阻断单独跟踪，不影响 Data 专项结果。
- M2-1 的 DTO/Port 注释统一使用中文；接口仍只覆盖当前真实消费者 Quote 与 DailyBars，PIT cutoff 继续明确表示 SystemObservedAt 修订时点。
- 下一步：M2-2 实现 DataService 到 Application Port 的映射适配，并以测试验证 nullable、整数精度、单位、来源版本、ProviderTime、复权、质量、fallback、系统观察时点不丢失。


### 2026-11-09：M2-2 DataService 到 Application Port 的映射适配

- 新增 `internal/marketdata/application.Reader`，通过窄接口包装既有 DataService 的 `Quote`、`DailyBars`、`DailyBarsAsOf`；不改变 Provider 选择、缓存、持久化和 fallback 规则。
- `GetQuote` 映射既有 `model.Quote` 到 `app.QuoteSnapshot`；`GetDailyBars` 根据 `SystemObservedAtCutoff` 显式选择当前视图或历史修订视图。
- 映射完整保留 nil 指针、int64 成交量、OHLC/均线、单位、复权方法/基准日/因子来源与版本、ProviderTime、FetchedAt、NormalizedAt、SystemObservedAt、质量、fallback、RequestID 和 normalization version。
- 指针与 slice 在边界处深拷贝，避免消费者修改 Application DTO 反向影响 DataService 返回对象。
- 汇总质量沿用 HTTP Handler 的现有判定：任意质量降级 warning 或任意非 valid bar → degraded；空数据集 → unknown；仅信息性 `bars_truncated_to_limit` 不单独降级。
- PIT 参数只使用 `SystemObservedAtCutoff`，原样传入 `DailyBarsAsOf`；不把 Analysis.AsOf、交易日期或 ProviderTime 混为一谈。
- 新增 fake backend 单元测试覆盖 Quote nullable/int64/来源/质量/fallback、当前日线与 PIT 路由、修订时点透传、单位/因子元数据及深拷贝。


### 2026-11-09：M2-2 DTO 字段来源校对与 CI SHA 顺序修正

- Application DTO 字段来源：`QuoteSnapshot` 逐字段映射 `model.Quote`；`DailyBar` 逐字段映射 `model.DailyBar`。`NormalizationVer` 在 Application 层命名为 `NormalizationVersion`，`AdjustmentRefDate` 命名为 `AdjustmentReferenceDate`；其余来源/时间/单位/质量/复权/fallback 语义保持不变。
- `DailyBarsQuery.Symbol/StartDate/EndDate/Limit/AdjustmentBasis` 是消费者输入；日期格式 YYYY-MM-DD、边界包含语义和 limit 归一化仍由现有 DataService 校验。只有 `SystemObservedAtCutoff` 非 nil 才调用 PIT Revision 查询。
- 删除 `DailyBarsSnapshot` 重复的顶层 VolumeUnit/AmountUnit：单位已逐条保存在 `DailyBar`，不从首行推导整个数据集单位，避免混合/空数据时产生错误汇总。
- `DailyBarsSnapshot.QualityStatus` 来源为 Data 现有 warning 判定与逐条 bar QualityStatus；`Warnings` 原样来自 DataService；`SystemObservedAtCutoff` 是请求 cutoff 的副本。
- CI 顺序调整：当前分支 Push 先完成根依赖 tidy 与 Data gofmt，再触发 workflow_dispatch 对最终 SHA 执行 Data、News MariaDB 验收；避免并行 Job 检查尚未 tidy/format 的旧 SHA。PR 验收仍保留，Docker build 仍只允许 integration 目标。


### 2026-11-09：M2-3 Data Bootstrap 组合根提取

- 新增 `internal/marketdata/bootstrap`，将 Provider 实例化、能力接口注册、DataService 装配、Application Reader 装配和 HTTP Handler 装配从 `cmd/market-data/main.go` 移入 Module。
- Module 对外提供 `Handler()`、`Application()`、`Diagnostics()` 和 `ProviderSummary()`；消费者无需接触 DataService、Provider 集合或 Repository。
- `Dependencies` 显式注入 Store、Cache、Logger。Store/Cache 由调用方创建并拥有；Module 只关闭自身创建的 BaoStock Client，避免重复关闭共享基础设施。
- 保留原 ProviderPriority、ProviderSettings 能力校验、股票搜索独立优先级、补充 Provider 和 35 个 HTTP Operation 的现有装配行为；不改变 Provider 算法、API DTO、路由或数据质量规则。
- 当前独立命令仍负责 DB/Redis 打开、MariaDB Migration、HTTP Server 生命周期及 Quote Retention Worker；下一任务 M2-4 再把 Worker Start/Stop/Wait 纳入 Module 生命周期。


### 2026-11-09：M2-3 Bootstrap 装配边界测试

- 新增 Bootstrap 单测覆盖 Handler/Application/Diagnostics 暴露、ProviderSummary、就绪检查、Summary 防御性拷贝、Close 幂等、缺失 MariaDB/Redis 注入和未知 Provider 配置拒绝。
- 独立命令改为使用 `bootstrap.New`，HTTP Server 仍由命令入口创建；Bootstrap 仅返回 Handler，不自行监听端口。
- 修正命令入口对 `http.Handler` 的调用方式；资源关闭顺序保持 Module 先关闭自有 BaoStock Client，再由命令入口关闭 Store。
- CI 格式化步骤扩展至 bootstrap 与 cmd/market-data，确保新组合根与命令入口执行 gofmt。


### 2026-11-09：M2-4 Module 生命周期与 Quote Retention Worker 收口

- 将 Quote Retention Worker 从命令入口移入 Bootstrap Module；Worker 改为同步循环，由 Module goroutine 启动并持有 cancel/done。
- 新增 `Start(ctx)`、`Wait(ctx)`、`Stop(ctx)`、`Close(ctx)`：Start/Stop/Close 幂等；Stop 取消并等待；Wait 只等待不取消；Close 等 Worker 退出后关闭 BaoStock 自有资源。
- Worker 保留原规则：Retention 小于 24 小时或无 Store 时不启动；启动立即执行一次清理，之后每 24 小时执行；单次数据库操作 30 秒超时。
- Store/Cache 仍由调用方持有。Close 超时会返回错误并保留可重试状态，不会提前关闭共享 Store。
- 独立命令不再直接启动 Worker；在 HTTP Server 启动前调用 Module.Start，退出时使用独立 35 秒 Close Context，确保 Worker 先退出再由命令入口关闭 Store。
- 新增生命周期测试覆盖重复 Start/Stop/Close、Wait 超时、Stop 后重启、Close 后拒绝 Start。


### 2026-11-09：M2-4 生命周期实现细节校对

- `Module.Start` 将 Retention Worker 绑定到调用方 Context；`Wait` 只等待当前 Worker，不触发取消；`Stop` 取消并等待，允许 Stop 后再次 Start。
- `Close(ctx)` 使用独立关闭锁与生命周期锁，阻止关闭过程中重新 Start；若等待 Worker 超时，保留可重试状态，不关闭 BaoStock Client，也不释放调用方 Store/Cache。
- 命令入口在退出时为 Close 创建 35 秒独立 Context，避免复用已经取消的进程 Context。
- `ProviderSummary` 字段来源为已通过 ProviderSettings/ProviderPriority 校验并实际注册的 Provider `Name()`；Quotes/DailyBars 分别来自对应能力集合。


- 生命周期补充边界：Close 等待超时后 Module 保持 `closing` 状态，禁止重新 Start；调用方可释放阻塞任务后重试 Close。新增忽略 Context 的阻塞 Store 测试验证该行为，避免关闭超时后后台任务被意外重启。


### 2026-11-09：M2-4 Data 专项 CI 验收

- 验收 Run：`42571224072`，提交 SHA：`042c40c8ff408d2bdc6a07d5a10c06851ef28586`。
- Data gofmt、go test（含 Application/Bootstrap 生命周期测试）、race、vet 全部通过。
- Data MariaDB Migration/PIT、News MariaDB 集成、go-core 全部通过。
- Root 全仓 `Test .` 仍在 gofmt 阶段失败，准确阻断文件为 Research 所属的 `internal/research/context/validate.go`（非法反斜杠）和 `internal/research/evidence/authorization.go`（NUL 字节）。本分支不越权修改 Research。
- Docker 构建按用户约束跳过；没有合并或修改 integration。
- M2-1 至 M2-4 已完成：消费者 Port、DataService 映射、Bootstrap 装配、Module 生命周期。M2-5 HTTP Adapter 接口解耦仍待完成；当前 HTTP Handler 仍通过旧构造器接收具体 DataService 与能力 Provider 集合。


### 2026-11-09：M1-2 迁移后 Go/Python 质量契约 CI 路径修正

- 当前 SHA：`42e2b9fbd0f87883b9705c5b3dea0f036322f112`；对应 PR Run `42571532273` 的 Go-Python parity Job 失败于仍使用旧目录 `services/market-data/internal/normalize`，并非质量契约测试断言失败。
- 已确认迁移后的唯一代码位置为 `internal/marketdata/normalize`，且同 SHA 的 Data 根模块 gofmt/test/race/vet 全部通过。
- 修正 `.github/workflows/data-quality-parity.yml`：push/PR paths、Go cache 依赖文件、Parity 测试命令、Data 测试命令和契约测试文件检查统一指向根模块路径。
- Data 全量测试限定为 `go test ./internal/marketdata/...`，避免把其他模块纳入 Data 专项门禁；不修改 Python/Go 数据质量判定规则、容差、样本或生产代码。
- 真实 Provider 数据验收仍需具备双方可审计导出及来源证据；本次 CI 路径修复不构成真实数据一致性验收。
- WebUI 检查失败仍因 WebUI 测试引用旧 Data 文件路径，属于 WebUI Owner 范围，本分支不修改 WebUI 文件。
- 下一步：完成 M2-5，让 HTTP Handler 依赖 Application Port；保持 HTTP DTO、路由、认证、错误语义与现有 35 个 Operation 不变。


### 2026-11-09：M2-5 HTTP Adapter 用例接口解耦（第一阶段）

- `httpapi.Server` 不再持有 `*service.DataService`，改为依赖本地 `marketDataApplication` 接口；接口只包含当前 HTTP Handler 实际调用的 17 个既有用例：Quote、DailyBars/PIT、Bundle、市场概览/资金流/板块/龙虎榜、股票资料/搜索/公告、Provider Diagnostics 和 Ready。
- 接口以 `service.DataService` 编译期断言验证实现完整性；未新增业务方法、外部 API、DTO 字段或 Provider 调度规则。
- HTTP Adapter 不再在请求处理时读取 `SupplementProviders()`；补充 Provider 的来源能力状态在 Server 构造时一次性快照到 `SourceStatus`，保持原响应内容与排序。
- Provider 实例化、能力注册、PriceLimitEvent Provider 注入仍由现有 Bootstrap/兼容构造器完成；HTTP Handler 不拥有 Provider/Repository/Cache。
- 保留现有 `New`、`NewWithCapabilities`、`NewWithConfig`、`NewWithCapabilityInterfaces` 构造入口，避免本阶段改变 35 个 Operation 的装配和测试方式。该阶段只收紧 Handler 的运行时依赖边界，不宣称已经完成 Server 根组合和所有 Provider 元数据构建迁移。
- 验收标准：Data HTTP API Contract、OpenAPI、认证/Request ID/错误响应、全部 HTTP Handler 测试不变；Data gofmt/test/race/vet 通过。
- 下一步：M2-5 第二阶段评估 Bootstrap 直接构造来源状态并调用接口构造器，逐步移除 HTTP Adapter 构造器中的 DataService/Provider 具体依赖；不得为此扩大公共 `app.MarketDataReader`（Analysis 只需要 Quote/DailyBars）。


### 2026-11-09：M2-5 Application 注入构造入口与来源快照测试

- 新增 `httpapi.NewWithApplication`，允许 Bootstrap 注入满足现有 HTTP 用例接口的实现，并传入已计算的 `SourceStatus` 快照；不要求调用方提供 Provider 集合。
- 原有构造器保留为兼容入口，继续负责从当前 DataService/Provider 配置生成来源能力状态，再委托新构造器；现有调用点行为不变。
- `NewWithApplication` 对来源状态切片执行防御性复制，并沿用原有 maxBars/requestTimeout/logger 默认值。
- 新增测试使用仅实现接口形状的 Stub 构造 Server，验证无需具体 DataService/Provider 即可装配 HTTP Adapter，且构造后的来源快照不受调用方切片修改影响。
- 当前 Bootstrap 仍使用兼容构造器；将其切换到新构造入口并迁移来源状态生成逻辑作为 M2-5 第二阶段，不在本批混入。


### 2026-11-09：M2-5 HTTP Handler 按用例域拆分接口

- 将单一 `marketDataApplication` 拆为 Quote、MarketOverview、Security、Fundamentals、Index、Trading、Diagnostics 七组接口；Server 不再持有 `*service.DataService`，也不扩大 Analysis 使用的公共 `app.MarketDataReader`。
- 将全部 HTTP Handler 中的 `s.svc`/旧 `s.app` 调用迁移到对应接口字段；接口签名与现有 DataService 方法一致，不新增业务方法、路由、DTO 或数据字段。
- 新增编译期断言，确认当前 DataService 实现七组接口；`ProviderProbeOptions` 定义移至 `model`，`service.ProviderProbeOptions` 保留类型别名，HTTP Adapter 不再为探测请求类型导入 service。
- `NewWithApplication` 改为接收 `ApplicationDependencies`，按用例域注入，避免再次引入单一巨型接口；旧构造器继续兼容现有 Bootstrap，来源能力快照仍由原构造逻辑提供。
- 现阶段尚未完成 M2-5 第二阶段：Bootstrap 仍调用兼容构造器，下一步将 Provider 能力来源状态构建迁入 Bootstrap，并让生产装配直接调用 `NewWithApplication`。


### 2026-11-09：恢复 Provider Probe 原有模型定义

- 接口拆分时新增的 `ProviderProbeOptions` 与原有 `model/provider_probe.go` 同路径，初次提交误覆盖了原有 ProviderProbeRun、CheckResult、FieldComparison、Item 等诊断模型。
- MariaDB/PIT CI 明确报出这些类型缺失；已从接口拆分前的提交 `d0487f98` 恢复原文件全部内容，并在原文件末尾追加 `ProviderProbeOptions`。
- 这次修复不改动任何原有诊断字段、JSON 名称或持久化语义；后续模型新增先检查目标路径，禁止覆盖已有模型文件。


### 2026-11-09：M2-5 用例分组接口编译缺陷修复

- 最新分组接口提交 `5f2f6520` 后，CI 明确发现旧 `s.svc` 字段残留在诊断 Handler 的 nil 判断；改为检查独立注入的 `DiagnosticsApplication`，无诊断实现时保持原有空诊断响应。
- Provider Probe 请求 DTO 已归属 `model.ProviderProbeOptions`；HTTP Adapter 改为直接使用 model 类型并移除不必要的 service 包导入，`service.ProviderProbeOptions` 兼容别名保留。
- 恢复 Provider Probe 领域模型的后续提交 `3b24b086` 已补回 Run/Item/CheckResult/FieldComparison 等原有模型定义；本次不重复修改该文件。
- 新增缺失 DiagnosticsApplication 时的 HTTP 回归断言；不改接口字段语义、Provider 探测规则或持久化结构。


### 2026-11-09：M2-5 分组接口命名与测试适配

- 修复七组 Application 接口接入后的字段命名冲突：`Server.fundamentals` 与 `fundamentals()` Handler 同名，统一改为 `fundamentalsApp`，基本面与估值 Handler 使用同一注入接口。
- 更新价格限制规则单测的 Server 装配字段，从旧 `svc` 改为 `trading`；测试仍验证缺少可信 Provider 证据时返回 unknown，不改变 fail-closed 规则。
- 清理 Provider Probe 模型文件多余空行，修复 Data 专项 gofmt 门禁。
- 上一轮 CI 已确认 Provider Probe 模型定义恢复后，MariaDB 编译缺失类型问题已定位到旧提交；全仓 gofmt 仍被 Research 两个文件阻断，归属不变。


### 2026-11-09：M2-5 第二阶段——Bootstrap 直连 Application 接口

- Bootstrap 不再调用 `NewWithCapabilityInterfaces` 兼容构造器；生产装配直接传入七组 Application 用例接口、来源能力快照和超时配置调用 `NewWithApplication`。
- Provider 能力状态的输入来自 Bootstrap 已通过配置校验并实际注册的能力接口切片；新增 `httpapi.SourceCapabilities` / `BuildSourceStatuses` 仅将这些真实注册能力映射为既有 `model.SourceStatus`，不读取配置猜测、不执行探测、不新增响应字段。
- 补充 Provider 的 Chip/CapitalFlow 状态仍按原规则生成；历史指数成分能力仍通过实际 Provider 是否实现 `HistoricalIndexConstituentProvider` 判定。
- `PriceLimitEventProvider` 的注册从 HTTP Adapter 兼容构造器迁回 Bootstrap；只在实际配置 Provider 存在该能力时注入 DataService，保持原有调用顺序和能力语义。
- 兼容构造器继续保留给现有测试和旧调用点，并复用同一个 `BuildSourceStatuses` 映射逻辑，避免重复维护能力矩阵；当 `svc == nil` 时保持 nil Application 依赖，不制造 typed-nil 接口。
- 新增来源能力映射测试，验证已注册能力为 true、未注册能力保持 false；不增加新 Provider、API、字段或诊断规则。
- 验收：Data gofmt/test/race/vet、HTTP API Contract 和 MariaDB/PIT 集成通过；全仓门禁继续单独记录 Research 语法阻断。


### 2026-11-09：M2-6 暴露 Data Migration Runner

- 代码复核发现 Data Migration 仍由独立命令直接调用 `repository.Store.Migrate`，Bootstrap Module 尚未向 Root Server 暴露迁移能力，不满足 M2“模块返回 Migration Runner”的组合契约。
- 新增 `bootstrap.MigrationRunner` 窄接口及 `Module.MigrationRunner()`；底层仍复用 Data Repository 已有的 `Store.Migrate`，不复制 SQL、不新增迁移框架、不改变 Schema。
- 未配置 Store 时返回 nil，表示当前部署没有 Data 持久化迁移任务；不构造 typed-nil 接口。
- 独立兼容命令改为先构造 Module，再从 Module 获取 Runner 并按配置显式执行迁移；迁移失败时通过既有 Module Close 释放模块自有资源，Store 仍由命令持有并关闭。
- Root Server 后续可在自己的 Migration Registry 中注册该 Runner，决定与 Analysis/其他模块的执行顺序；Data 仍拥有迁移内容，Platform 不接管业务表和 SQL。
- 新增无存储配置时 MigrationRunner 返回 nil 的单测；Repository 的迁移幂等和 MariaDB/PIT 集成测试继续作为迁移行为证据。
- 当前 SHA `33760950` 的 Data Module gofmt/test/race/vet、MariaDB/PIT、News MariaDB、Platform、Live Probe、Go/Python parity 均通过；Root 全仓 gofmt 仍被 Research 文件阻断，WebUI 门禁失败归 WebUI Owner。本次修改后的结果以新 SHA CI 为准。


### 2026-11-09：M4-DB 前置切片——Data Repository 支持借用 Root Pool

- M3 Root Server 装配需要把 Root Store 的 `*sql.DB` 注入 Data Repository；现有 `OpenMySQL` 总是创建独立 Pool，直接复用会造成重复连接池和重复关闭风险。
- `repository.Store` 增加明确的 `ownsDB` 所有权标记；原 `OpenMySQL` 创建的 Pool 仍由 Store 拥有，`Close` 行为保持不变。
- 新增 `NewWithBorrowedDB(*sql.DB)`，只包装调用方 Pool，不 Ping、不调整连接池参数、不关闭该 Pool；nil DB 返回明确错误。
- 该构造器只解决 Data Repository 的 DB 句柄所有权，不改 SQL、Schema、Migration 内容和查询语义；Root Server 仍负责创建、配置、Ping 和关闭 Pool。
- 已新增 borrowed/owned 关闭行为测试和 nil DB 入参测试。SHA `d82ab219` 的 PR Go suite Run `42577608692`：Data Module Job `129028747053` 的 gofmt、单测、race、vet 全部通过；MariaDB Migration/PIT Job `129028747005` 通过。Root `Test .` 仍因 `internal/research/context/validate.go` 非法反斜杠和 `internal/research/evidence/authorization.go` NUL 字符在 gofmt 阶段失败，归 Research 处理。下一步由 Root Server Bootstrap 注入 borrowed Store；Redis 暂不做接口替换，避免把两类资源迁移混成一批。


### 2026-11-09：M4-DB borrowed Pool 专项验收

- 当前代码 SHA：`d82ab2191354cd4fc696fb387042d81ce479c739`。
- PR Go suite Run `42577608692`：Market Data Job `129028747053` 的 Data gofmt、单测、race、vet 全部通过；MariaDB Migration/PIT Job `129028747005` 通过，覆盖现有 Data Migration、PIT Revision、DataService fallback 与 Provider Probe HTTP/MariaDB。
- 新增连接池所有权测试通过：borrowed Store.Close 不关闭调用方 Pool；owned Store.Close 关闭自有 Pool；nil DB 构造返回错误。
- Root 全仓 gofmt 仍仅被 Research 两个文件阻断；不在 Data 分支修复。
- M4-DB 的接口和所有权测试通过；Root Server 尚未实际注入 borrowed Store，因此不标记 Root/Data 共享 Pool 集成验收完成。


### 2026-11-09：M3-1 拆分 Data HTTP 业务路由与传输中间件

- Data HTTP Adapter 保留原 `Handler()` 作为独立部署入口：33 条业务路由仍逐条执行 Data API Key 认证，健康路由仍公开，外层 Request ID、Access Log、Timeout Context 和 Security Header 行为不变。
- 新增 `InProcessHandler()`，只注册业务路由，不包含 Data API Key、Data Request ID、Access Log、Timeout Context、Security Header，也不暴露 Data 独立健康路由；Root Server 必须在外层提供可信认证/授权、Request ID、超时和安全策略后才能挂载。
- `bootstrap.Module` 同时暴露兼容 `Handler()` 与 `InProcessHandler()`，两者由同一 HTTP Server/同一组 Application Dependencies 构造，避免双实例业务状态。
- 新增 `config.LoadForInProcess()`：只跳过 Data HTTP API Key 校验，其他 Provider Secret、配置范围、存储约束仍执行原有校验；独立部署的 `config.Load()` 继续强制要求 Data API Key。
- 新增路由边界测试：进程内业务路由无需 Data API Key，Request ID 不被 Data 覆盖，Data health 路由不对 Root 暴露；新增配置测试证明独立部署仍要求 API Key。
- 本批未修改 Root Server、认证中间件或 Platform 代码；尚未把 InProcessHandler 实际挂载到 Root Router，不计 Root 集成验收完成。


### 2026-11-09：M3-1 Data 进程内路由专项验收

- 实际验证代码 SHA：`7d7c3ee0401195839254a96716ad4502d6097596`（自动 gofmt 仅调整 Module 字段对齐）。
- Go suite Run `42578303159`：Data Module Job `129030926997` 的 Data gofmt、单测、race、vet 全部通过；MariaDB Migration/PIT Job `129030926876` 通过。
- 单测验证 `InProcessHandler` 不要求 Data API Key、不覆盖 Root Request ID、不暴露 Data health 路由；`Handler` 独立部署认证测试仍保留。
- Root `Test .` 在同一 Run 仍被 Research 的语法/NUL 问题阻断；Data 专项不受影响。
- 当前只完成 Adapter/Config 边界准备，尚未修改 `cmd/analysis-api`，也未实际将 Data Module 装入 Root Server；M3 总体验收不能标记通过。


### 2026-11-09：M4-Redis 共享 go-redis Cache Adapter

- 新增 `cache.NewGoRedis`，只依赖 Cache 所需的最小 Redis 命令接口，将 Root Server 已持有的 `*redis.Client` 适配为 Data 既有 `cache.Cache`。
- Adapter 不创建、不 Ping、不关闭 Redis Client；Root Server 负责连接池生命周期。Data 仍拥有 Cache key、TTL 和 fallback 语义。
- 保持现有 Cache 契约：GET 的 `redis.Nil` 映射为 cache miss；其他错误原样返回；SET 保留原始 value 和 TTL，低于 1ms 继续拒绝；Ping 透传依赖错误。
- 原自定义 RESP Adapter 保留给独立兼容部署，Root 同进程装配不再使用每条命令单独 Dial 的实现。
- 新增 fake go-redis 命令测试，覆盖命中/未命中、TTL、value 和 Ping 错误语义；不新增 Redis key、缓存字段或公共平台代码。
- Root 与 Data Redis 配置是否完全一致、是否复用同一 Client，由后续 Root Bootstrap 按 Addr/Username/Password/DB 明确判定；不匹配时由组合根创建并关闭独立 Client。


### 2026-11-09：当前 HEAD 与 M4 Redis 适配状态校准

- 当前分支 HEAD：`db69e4a17967e450a8b6cf1d7417df625c1bf0ea`；最新提交新增 `internal/marketdata/cache.GoRedis`，将 Root 已持有的 go-redis Client 适配到 Data 既有 `cache.Cache`。
- 适配边界：Data 不创建、Ping 或关闭 Redis Client；Root 负责 Client 生命周期。GET 的 `redis.Nil` 映射为 cache miss；其他错误透传；SET 保留既有 value/TTL 约束；不新增 Cache key、业务字段或 API。
- 最新 PR Go suite Run `42579001733`（PR merge SHA `b9aa94f7358f8698b2f74c9c8f4c568bb08c0c85`）：Data 根模块 Job `129033111561` 的 gofmt、tests、race、vet 全部通过；MariaDB Migration/PIT Job `129033111558` 通过。说明 Data 专项编译/测试没有被本次 Redis Adapter 阻断。
- 同一 Run 的 Root `Test .` Job `129033111530` 在全仓 gofmt 阶段失败，定位为 Research 文件 `internal/research/context/validate.go` 的非法反斜杠，以及 `internal/research/evidence/authorization.go` 的 NUL 字节；Root 测试/race/vet 因前置格式检查失败而 skipped。该失败不归属 Data，本分支不修改 Research。
- 同一批次 Docker/Compose Job skipped；因此本 SHA 没有新的镜像构建证据。WebUI checks 失败属于 WebUI Owner；Platform Go checks 与 Go/Python parity 通过。Live Provider workflow 外层 success 不自动代表历史 Provider 专项已运行或真实数据验收通过。
- M1/M2 的 Data 侧代码迁移与装配准备已具备；M3 尚未完成 Root Server 实际挂载，M4 尚未完成共享 Redis Client 实际注入及配置一致性验证。
- 下一步边界：
  1. 当前分支继续完成 Data 侧 M4：核对 `GoRedis` 与既有 Cache 行为的完整错误/TTL/禁用语义；只补真实缺陷，不增加第二套缓存抽象。
  2. Root 侧共享 Redis Client 创建、Addr/Username/Password/DB 匹配、借用或独立 Client 生命周期，由 Server 组合根负责；本分支不修改 `cmd/analysis-api` 或 `internal/server/**`。
  3. Provider HTTP 出站接入复用 `internal/platform/httpclient` 的 BoundDoer/DestinationPolicy；逐 Provider 明确 endpoint Host/Port、HTTP/HTTPS、重定向和请求体/响应限制。不得在 Data 重写 SSRF、DNS/IP 或连接池能力。
  4. BaoStock/Pytdx TCP 出站策略单独处理，不包装成 HTTP，也不扩大 HTTP Client 职责。
  5. M3/M4 代码集成验收仍按用户要求在 `origin/integration` 执行；本分支不执行构建、不修改 integration。


### 2026-11-09：M4-Redis Cache 契约回归补充

- 代码基线：`e25afee8d6e1c28b880d805d653edacd5073d9b8`。
- 在既有 `cache.Cache` 接口与 `GoRedis` 实现不变的前提下，补齐测试对 GET key 原样传递、GET 非 miss 错误透传、SET 错误透传的断言。
- 本次只增加回归覆盖，不新增接口、字段、缓存 key、TTL 规则或公共平台代码；既有 `redis.Nil` 仍是唯一映射为 cache miss 的错误。
- 本次提交后尚未执行构建或测试。按当前约束，不在功能分支执行构建；如需构建，须先按既定流程在 integration 验收，且不得直接修改 integration。
- 因此当前判定为：实现与测试代码已提交，新增断言待集成构建验证；不能标记 M4 Redis 完整验收通过。
- 下一步：Tushare 已增加可注入 `HTTPDoer` 的构造入口和 Bootstrap 依赖传递，等待 integration 验证；随后按 Provider 分组接入其余 HTTP Provider，不一次性重写全部 HTTP Provider。


### 2026-11-09：M4-HTTP 第一切片——Tushare 受控 HTTP 执行器注入

- 代码基线：`bbfd6dd90212fe6042093300016ab059b8a7561f`。
- Provider 层新增最小 `provider.HTTPDoer` 接口及 `NewTushareConfiguredWithHTTPDoer`；原 `NewTushareConfigured` 保留并委托新构造器，独立兼容部署仍默认使用标准 `http.Client`。
- Bootstrap `Dependencies` 新增可选 `TushareHTTPDoer`，仅用于将 Root 提供的受控执行器传给 Tushare；Module 不拥有、不关闭该执行器。未注入时行为与原来一致。
- Tushare 的日 K 与其他能力请求共用同一 `p.client.Do`，本批将两条调用路径统一切换为注入接口；未修改 Token JSON Body、API 名称、字段、频率限制、解析、复权和错误语义。
- 新增固定测试验证日 K 与能力接口都使用注入的 Doer，POST URL/Content-Type/API 名称/Token 传递保持不变；使用 502 响应验证原有上游错误路径仍可见。
- Root 组合根接入时必须针对实际 `TushareAPIURL` 创建 BoundDoer：精确 Host、HTTPS、443 端口、禁止重定向、`Idempotent=false`（Token 位于 POST Body）；Platform Client 的响应体上限需覆盖 Data 既有 Provider payload 上限。Root 负责创建和关闭 Platform Client。
- 本批尚未执行构建/测试；不宣称 Tushare 受控出口验收通过。必须在按约定集成后验证 Data gofmt/test/race/vet、Tushare 固定测试和 Platform HTTP Client 安全策略。
- 下一步：按凭据和重定向风险优先级，接入 Hithink、TickFlow、Xueqiu 等带凭据的 HTTP Provider；每个 Provider 的 endpoint Host/Port、凭据类型、重定向和响应上限独立登记。随后处理无凭据 Provider；BaoStock/Pytdx 仍单独制定 TCP 出站策略。


### 2026-11-09：integration 合并后基线复核与 M4-HTTP-02

- 用户将 `integration` 合并到当前分支，最新合并提交为 `be892d89f4247670efd1c64774d4866e03ccc454`，父提交为此前 Data 提交 `222b2c93039ad5df8ea0ed78f945692b504fc673`。本次合并信息显示 `go.mod` 存在冲突；合并后文件已恢复为单一根 Module（Go 1.23.0），依赖声明无冲突标记，包含当前仓库所需的 go-redis、mysql、jsonschema 等依赖。尚未通过构建验证，不能仅凭文件内容认定 go.sum/依赖图完全正确。
- 合并带入的最新 Platform HTTP Client 仍提供 `Client.Bind(DestinationPolicy)` / `BoundDoer`，可直接复用；没有创建 Data 自有 SSRF/HTTP 安全层的理由。
- 当前 Provider 核对：Tushare 已具备 HTTPDoer 注入；Hithink、TickFlow、Xueqiu 仍直接持有 `*http.Client`。Hithink 使用单一 baseURL + GET + `X-api-key`，适合按 Provider 注入固定 Host/Port 的 BoundDoer；本阶段只接 Hithink，不混做 TickFlow 的双 endpoint 或 Xueqiu Cookie 请求。
- M4-HTTP-02 代码切片：Hithink 新增 `NewHithinkConfiguredWithHTTPDoer`；保留原构造器兼容行为；Bootstrap Dependencies 增加可选 `HithinkHTTPDoer` 并透传。未注入时仍使用标准 `http.Client`。
- 新增测试只验证注入执行器被调用、GET URL/Accept/X-api-key 保持既有语义、502 错误仍可见；不输出或记录真实 API Key，不增加 API/字段/数据语义。
- Root 注入策略要求：按实际 `HithinkBaseURL` 解析并精确绑定单一 Host；默认 HTTPS/443，除明确的本地测试配置外禁止 HTTP；不允许重定向（携带 X-api-key）；GET 可按 Platform 幂等策略配置，但不能因重试导致超出 Provider 速率限制。Platform Client 响应上限必须覆盖现有 `maxProviderPayloadBytes`，超限判定由公共层完成。
- 本批提交后不在当前分支执行构建/测试；Hithink 受控出口仅完成注入接口和单测源码，待 integration 验收。下一步为 M4-HTTP-03 TickFlow：分别处理认证 API URL 与免费 API URL 的不同目的地址策略，不能共用一个无区分 BoundDoer。
