# Market Data 并入 Root Server 的模块化单体调整方案

- 维护分支：`feat/design_doc`
- 设计基线：`7344170dc53124361c9cf49efb0d66821d3605c1`
- 决策状态：`accepted`（采用同进程模块化单体方向）
- 实施状态：`not_started`（本文件只冻结目标架构与迁移顺序，不代表代码已改造）
- 范围：Root Server、Market Data、WebUI 部署接入、Platform 基础设施装配
- 非目标：本轮不修改 Data/Platform/WebUI 业务分支，不改变行情业务语义，不删除既有 Data API DTO/Schema，不拆分 Provider 子模块

> 本方案落实项目总规划中“Server 是唯一组合根、业务模块优先同进程公开 Port、确有部署隔离需求再拆服务”的原则。合并的是运行时和 Go Module，不是 Data 的领域 Owner、Repository 或业务规则。

---

## 1. 决策摘要

采用方案 B：**Market Data 与 Root Server 合并为同一个 Go Module、同一个进程、同一个部署单元；Data 保留独立领域模块和可替换的 HTTP Adapter。**

目标运行结构：

```text
Browser / WebUI
      |
      | same-origin HTTPS
      v
Root Server :8092
  |-- Server Bootstrap / Router / Security / Request Context
  |-- Analysis Application
  |-- Market Data Module
  |    |-- Provider Adapters
  |    |-- Data Application / Use Cases
  |    |-- Domain Model / Normalization / Quality / PIT
  |    |-- Repository / Cache Adapter
  |    |-- HTTP Adapter (兼容与外部 API 边界)
  |-- Research / News / Quant / Researchflow / LLM / Notification
  |
  +-- shared Platform infrastructure
       |-- MariaDB connection pool
       |-- Redis connection pool (按模块可选)
       |-- controlled outbound HTTP client
       |-- config / secret / diagnostics / request context
```

部署目标：

```text
docker-compose
  |-- stock-analysis-server  (Root Server + Market Data)
  |-- stock-analysis-web
  +-- external MariaDB / Redis / 1panel-network
```

不再默认部署 `stock-analysis-data` 容器。保留独立 Data HTTP Adapter 的能力，作为兼容接口、诊断入口或未来拆分时的传输层，不再作为 Server 内部调用的必经路径。

### 1.1 决策理由

1. 当前部署是单机 Compose，Server 与 Data 没有独立扩缩容或多租户隔离需求。
2. Server 已是唯一组合根；News 等业务模块已按同进程 Application Port 规划。
3. Server 内部调用 Data 目前经 HTTP、Data API Key、Base URL 和 JSON DTO 往返，增加不必要的网络、配置和认证维护成本。
4. Data 的 Provider、质量、PIT、Repository 和采集生命周期仍有清晰领域边界，适合保留为模块而不是独立服务。
5. 同一进程可以共享基础设施资源并统一生命周期，未来拆分时只需替换模块 Adapter，不必重写领域逻辑。

### 1.2 不选择其他方案

| 方案 | 结论 | 原因 |
|---|---|---|
| Data 继续独立部署 | 暂不采用 | 对当前单机运行没有足够收益，持续承担内部 HTTP、Secret、健康检查和部署编排成本 |
| 一个容器两个进程 | 不采用 | 仍保留 HTTP、认证和双进程监管，复杂度没有实质下降 |
| Data 代码直接揉入 Server Application | 禁止 | 会破坏领域 Owner、依赖方向和未来拆分能力 |
| 同一进程模块化单体 | 采用 | 统一部署与生命周期，同时保留模块边界和未来拆分能力 |

---

## 2. 当前代码事实与审计发现

以下结论基于设计分支当前可读取的文件，不等于已完成所有依赖的静态分析或运行时测试。

| 位置 | 当前事实 | 对合并的影响 |
|---|---|---|
| 根 `go.mod` | Go 1.22；模块路径 `example.com/stock-analysis-flow` | 需要统一模块路径和 Go 版本 |
| `services/market-data/go.mod` | 独立 Module，Go 1.23.0；包含 TDX、x/text 等依赖 | 不能直接 import Data 的 `internal` 包；合并后需要处理依赖版本 |
| `cmd/analysis-api/main.go` | Bootstrap 创建 MySQL/Redis Store、Market Data HTTP Client、Runner、Task Queue 和 HTTP API | 是当前 Root Server 进程入口；应演进为唯一组合根 |
| `internal/adapters/marketdata/client.go` | 使用 Base URL、HTTP Client、X-API-Key 调用 Data HTTP API，解析 JSON | 是 Server→Data 的网络边界，未来应替换为进程内 Port Adapter |
| `services/market-data/cmd/market-data/main.go` | 集中加载 Data Config、连接 MariaDB/Redis、构造 Provider 能力集合、DataService、HTTP Handler 并监听端口 | Data 的构造装配逻辑应提取为可复用 Module Bootstrap，不再由独立 main 独占 |
| `services/market-data/internal/httpapi/server.go` | HTTP Server 持有 API Key 校验、Request ID、超时、访问日志、安全 Header 与多条 API 路由 | Handler 与业务 Application 需要解耦；不能直接把原 Handler 当作不受保护的内部 Port |
| `services/market-data/internal/repository/mysql.go` | Data 自有 MariaDB Repository、表结构和迁移逻辑 | Data 继续拥有 Repository 和 Migration；连接池由组合根统一提供 |
| `services/market-data/internal/cache` | Data 自有 Cache 接口及 Redis Adapter | Cache 仍归 Data Adapter；Redis 连接生命周期应由 Server 统一装配或由 Data Adapter 明确 Close |
| `docker-compose.yml` | 独立 Data、Server、Web 三个服务；Server depends_on Data healthy | 移除 Data 服务后调整健康依赖和配置 |
| `web/default.conf.template` | `/market-api/` 代理 Data:8091，`/analysis-api/` 代理 Server:8092 | 两个 API 前缀改为同一 Root Server 上游，保留 URL 兼容 |
| `doc/整体任务规划/跨模块契约目录与冻结状态.md` | Server 是唯一组合根；News 已明确采用根 Module 同进程 Application Port | Data 应统一遵循该部署原则；逻辑依赖 DAG 不因同进程而放宽 |

### 2.1 当前最重要的技术风险

- **Go Module 版本：** 根模块 Go 1.22，Data 模块 Go 1.23.0。建议统一到 Go 1.23.x（以 Data 依赖与 CI 实际兼容结果为准），并同步 Docker Builder、CI、开发环境说明；不要为了维持旧版本而降级 Data 依赖。
- **构造函数耦合：** Data 目前的大量 Provider capability 装配集中在独立 main 中。必须抽成可测试的 `marketdata.Module` 或等价 Bootstrap API，返回 Application Port、HTTP Handler、Diagnostics 和 Close 生命周期句柄。
- **内部包边界：** 合并后 Data 代码迁入根 Module 的 `internal/marketdata`，根 Module 内其他模块可通过明确的公开 Port/Facade 调用；禁止消费者依赖 Data 的 provider/repository/model 私有实现。
- **共享资源：** 根 Server 与 Data 当前各自打开数据库连接，并可能各自执行 Migration。合并后不能简单让两个 Store 各开一套连接池和重复执行迁移；应由 Server 提供共享连接池，Data 只接收连接句柄并执行自己命名空间下的迁移。
- **HTTP API 安全：** Data 原 HTTP Handler 通过 X-API-Key 保护业务接口。浏览器不得继续持有 Data API Key；同进程不等于可以绕过 Server 用户/Owner 认证。外部 HTTP 路由必须由 Root Server 统一认证、授权、限流和 Request ID 管理，再调用 Data Application Port。
- **数据库兼容：** Data 当前表结构、索引、版本表、历史 PIT 数据和 MariaDB Migration 必须原样保留；本次不做表重命名、数据搬迁或历史数据清洗。

---

## 3. 目标模块边界

### 3.1 Market Data 模块拥有的事实与能力

Data 是以下事实的唯一 Owner：

- Provider 配置、Provider 能力、Provider 调用与来源状态。
- 行情、日线、指数、证券基础信息、财务、估值、日历、行业、资金流等 Data 领域数据。
- 数据标准化、单位/精度、复权口径、质量校验、覆盖率和降级语义。
- Snapshot、来源版本、系统观察时间、PIT 查询和数据质量诊断。
- Data Repository、Data 表结构与 Migration 内容。
- Data 采集/保留 Worker 的业务规则与执行函数。

Server 不得直接操作 Data 表、SQL、Provider 或 Data Repository。

### 3.2 Root Server 拥有的职责

- 唯一进程入口、配置聚合、依赖注入、HTTP Router、进程信号和优雅关闭。
- 共享 MariaDB/Redis/HTTP Client 等基础设施的创建与生命周期。
- 模块装配、跨模块公开 Port 注入和 Worker 启停顺序。
- 对外 API 的认证、授权、访问策略、限流、Request ID 和 Fault 映射。
- 不拥有行情领域对象、Provider 策略、数据质量规则或 Data SQL。

### 3.3 允许与禁止的依赖

允许：

```text
Server Bootstrap -> Market Data Module Constructor
Analysis Application -> Market Data Public Port
Researchflow -> Data Public Application Port
Data HTTP Adapter -> Data Application Port
Data Application -> Data Domain / Repository Port
Data Repository Adapter -> shared DB handle
```

禁止：

- Analysis/Research/Quant 直接 import `internal/marketdata/provider`、`repository`、`model` 等实现包。
- 业务模块直接查询 Data 表或复用 Data Repository。
- Data 反向依赖 Server、WebUI、Researchflow 或 Quant。
- Data 自己创建第二个 HTTP Server、第二套共享 DB Pool 或进程级 Logger。
- 同进程调用绕过 Application Port，直接访问其他模块内部对象。

---

## 4. 推荐代码结构

合并后建议按职责迁移，而不是机械地把整个 `services/market-data` 目录移动到任意位置：

```text
cmd/
  server/
    main.go                       # 唯一正式运行入口

internal/
  server/
    bootstrap/                    # 唯一组合根
    transport/                    # Root HTTP Router、认证与 API 前缀
    config/                       # 进程级配置聚合
  marketdata/
    app/                           # 对外 UseCase / Application Port
    domain/                        # 领域模型与规则（从现有 model/service 分层）
    provider/                      # Provider Adapter
    repository/                    # Data Repository Adapter
    cache/                         # Data Cache Adapter
    normalize/                     # 标准化与质量校验
    httpapi/                       # Data HTTP Adapter（只依赖 app Port）
    config/                        # Data 专属配置 Schema/Loader
    bootstrap/                     # Provider/Repository/Worker 装配
  adapters/
    storage/                       # Platform 共享存储 Adapter（不承接 Data SQL）
  platform/
    ...                            # 现有公共技术原语
```

结构是目标边界，不要求一次性重命名所有包。第一阶段可以先保留原有 package 名称与相对目录，仅完成 Go Module 归并和构造装配提取；在可编译、可测试后，再按需要做领域分层重构，避免把“合并部署”扩大成无关重写。

### 4.1 Data 对外 Port

Data 至少提供两类明确接口：

1. **业务 Application Port：** 供 Analysis、Researchflow、Research 等模块使用，返回强类型结果，不依赖 HTTP DTO。
2. **HTTP Handler Adapter：** 供 WebUI 兼容路由或受控外部 API 使用，只负责请求解析、认证上下文映射、响应编码和错误映射；内部仍调用同一个 Application Port。

不得把 Data HTTP DTO 当作所有同进程模块共享的 Domain 类型。

### 4.2 当前 Analysis 调用点迁移

当前 `internal/adapters/marketdata.Client` 是 HTTP 客户端，提供行情和日线读取能力。目标是新增/替换为进程内 Adapter：

```text
Analysis Runner
    |
    v
MarketDataPort
    |
    v
InProcessMarketDataAdapter
    |
    v
Market Data Application
```

迁移时保留当前调用方真正使用的行为：证券代码规范化、日线数量范围、复权口径、质量状态、来源版本、Provider 时间、抓取时间、单位、错误语义和 PIT 约束。不能因从 JSON 变为 Go 对象而丢失字段或把缺失值转换为零值。

原 HTTP Client 可在兼容期保留为 `RemoteMarketDataAdapter`，供独立部署回滚或未来拆分使用；待内部调用和部署验收完成后，再评估是否删除。不要同时让同一生产调用点在进程内与 HTTP 两种路径间隐式 fallback。

---

## 5. 认证、授权与 Request ID

### 5.1 Data API Key 的调整

同进程内部调用不需要 Server 给自己发送 HTTP，也不需要 Data API Key 作为模块间凭据。目标规则：

- 进程内调用通过 Server Bootstrap 注入的强类型 Application Port，不传递 `X-API-Key`。
- Data API Key 不再作为 Server→Data 内部调用的必要 Secret；完成切换后从 Server Config、Compose 和部署必填项中移除。
- Provider Token 仍只由 Data Config/Provider Adapter 持有，绝不暴露给 Server Application 或 WebUI。
- 如果保留独立 Data HTTP Adapter 供外部网络调用，则该 Adapter 必须有独立明确的认证策略；不能因为内部 Port 不需要 Key 就把原 HTTP 业务 API 改为匿名访问。
- 如果 Data HTTP API 仅通过 Root Server 暴露，Root Server 必须在路由层执行 owner/session/service 认证和对应 Scope 授权，再将可信 Principal 映射到 Data Application 请求上下文。不能让浏览器直接请求 Data Handler，也不能把客户端自报 Principal 当作可信身份。

因此，`DATA-CTR-006` 的“Server 通过 X-API-Key 调用独立 Data 服务”只适用于远程部署 Adapter。采用本方案后，需在契约登记簿明确其远程兼容范围与进程内调用规则，不应继续要求同进程内部调用携带 Data API Key。

### 5.2 DATA-CTR-008 与领域授权必须保留

取消 Data 服务间 API Key 不代表取消 Data 的数据授权：

- caller principal 必须由 Root Server 已验证的认证上下文或受信任模块身份产生。
- purpose_code、Scope、请求范围绑定、Snapshot Manifest 内容寻址和 AuthorizationRef 签发语义保持不变。
- `subject_ref` 不得由请求体、Query 或任意 Header 自报。
- Data 读取受限快照时仍需执行 Owner 当前授权复核；撤销、过期、unknown、依赖不可用和 TOCTOU 仍然 fail closed。
- Data API Key 不写入 AuthorizationRef、Manifest、fingerprint 或日志。
- 同进程调用必须保留审计主体、purpose、resource_ref、Request ID 和授权决策证据。

### 5.3 Request ID

- Root Server 在可信入口生成/校验 Request ID，并写入标准 Request Context。
- 进程内 Port 调用通过 context 原样传播同一个 Request ID，不重新生成无关联 ID。
- Data 的日志、诊断、错误和 Snapshot/审计记录按契约保留 Request ID / originating_request_id / correlation_id 的作用域。
- Request ID 不是身份或授权凭据。
- 未来重新拆分 Data 时，HTTP Adapter 从同一 Context 传播 Request ID，恢复跨进程关联。

---

## 6. 数据库、Redis 与 Migration

### 6.1 MariaDB

- MariaDB 仍是事实源；本方案不更换数据库、不迁移历史数据、不改变表结构。
- Server Bootstrap 创建共享 SQL 连接池，Data Repository 接收受控 DB handle；Data 仍拥有所有 Data SQL、Repository 和 Migration 内容。
- 各模块使用独立 Migration namespace/version；Server 统一按明确顺序调用各模块 Migration Runner。
- Migration 只能由组合根执行一次；禁止 Server Store 和 Data Store 各自自动执行同一批迁移。
- Migration 失败必须阻止 readiness/启动完成；不能继续接受请求并把缺表问题留到首次调用。
- 连接池的 MaxOpen/MaxIdle/ConnMaxLifetime 应按总负载统一配置，模块不能各自重复创建池或覆盖全局参数。
- 需要验证 MariaDB Driver 版本、时区、ParseTime、字符集、连接参数和现有表兼容。

### 6.2 Redis

- Redis 是可选缓存/协调基础设施，不得仅因同进程就升级为所有模块的强依赖。
- Server Bootstrap 可创建共享 Redis Client/Pool；Data Cache Adapter 接收接口或客户端句柄。
- Data 缓存 key 前缀、TTL、序列化和失效规则仍由 Data 拥有。
- Redis 不可用时的行为必须沿用当前 Data Config：明确 fail-fast、降级或禁用缓存，不得在合并时改变。
- Server 负责最终 Close；Data 负责停止使用资源，不重复关闭共享客户端。
- 若模块未来需要独立 Redis DB/连接参数，作为 Adapter 配置隔离，不等于必须独立部署。

### 6.3 Data 后台任务

- Quote Retention Worker、Provider Probe/诊断及其他 Data 定时任务由 Data 模块提供 Start/Stop 生命周期。
- Server 统一启动 Worker，并在 HTTP Server Shutdown 前停止接收新任务、等待执行中任务 drain。
- 每个 Worker 必须响应 context 取消、设置超时、避免重复启动；启动失败需回滚已启动资源。
- 不得由 Data 模块自行安装第二套进程信号处理器。

---

## 7. HTTP 路由与 WebUI 迁移

### 7.1 路由目标

当前 WebUI 使用两个同源前缀：

- `/analysis-api/` → Server :8092
- `/market-api/` → Data :8091

目标保留前端可见路径，统一由 Nginx 转发至 Root Server :8092：

- `/analysis-api/` → Root Server Analysis Router
- `/market-api/` → Root Server 挂载的 Market Data HTTP Adapter

这能降低 WebUI 改动范围，并避免在同一轮同时重写所有前端 API URL。路由迁移必须保证 path strip/prefix、Query、方法、状态码、响应体和超时语义不变。

### 7.2 认证要求

- `/market-api/` 不再直连 Data 容器。
- 浏览器不得持有 Data API Key；删除 WebUI 的 Data Key 输入、localStorage 读写和 X-API-Key 请求头。
- Root Server 在 `/market-api/` 路由边界执行现有 WebUI 所需的认证与授权。若现有前端依赖 Owner Token，应由 Server 认证机制处理，不能把 Data Key 替代为 Owner Token。
- 不允许把客户端提交的 X-API-Key 透传给 Data Handler。
- Data Handler 若继续保留旧 Key middleware，只能用于独立远程部署 Adapter；Root Server 同进程挂载时必须使用明确的受信任内部装配模式，不能通过空 Key/默认匿名绕过安全。
- 认证/授权失败必须与 Provider/数据源故障区分；健康检查不得暴露 Secret。

### 7.3 Nginx 与 Compose

- Nginx 的 `MARKET_DATA_SERVICE_NAME` 改为 Root Server 服务名，或改为统一 `ANALYSIS_SERVICE_NAME`，避免两个服务变量指向同一目标造成误导。
- Compose 移除 `stock-analysis-data` 服务、Data 独立端口 expose、Server 的 Data depends_on 和 Web 对 Data 健康状态的依赖。
- Root Server 容器负责 Data 配置、Provider Token、DB/Redis 依赖和 Data Worker。
- Web 只依赖 Root Server healthy。
- Server readiness 应纳入必须的模块装配和必要依赖状态；Provider 单个上游不可用不应自动导致进程不健康，遵循 Data 质量/诊断契约。
- 对外只暴露 Web 端口；Server 继续仅在受控网络或 loopback 暴露，不把 Data 业务端口独立暴露。

---

## 8. 迁移阶段与实施任务

迁移按阶段进行，每阶段可独立 Review 和回滚；不允许一次性大搬迁后才第一次编译。

### M0：依赖审计与冻结清单

Owner：项目总规划 + Server + Data

交付：
- Data 全量 import 依赖与 Go 版本清单。
- Provider 构造、DataService 构造、Repository/Cache 构造和所有后台 Worker 清单。
- 所有 Data HTTP 路由、WebUI API 使用点和 Nginx 转发规则清单。
- Data 表/索引/Migration namespace 与现有 DB pool 配置清单。
- DATA-CTR-006/008、PLAT-CTR-005/006/007、SRV-CTR-001 受影响点清单。
- go-core 入口与 Compose 正式入口关系的明确结论；不得只按目录名称推断是否仍是生产入口。

验收：每个迁移对象有当前路径、目标路径、Owner、兼容要求和测试证据；无未登记的 Data HTTP 调用点。

### M1：Go Module 归并（不改变部署调用）

Owner：Data + Server

交付：
- 将 `services/market-data` 代码纳入根 Go Module，迁移到 `internal/marketdata`。
- 统一 Go 版本与依赖；更新 go.mod/go.sum、Docker Builder、CI。
- 删除嵌套 go.mod/go.sum 和独立 Data Docker Build（仅在确认无其他开发流程依赖后）。
- 暂时保留原 Data HTTP API 与远程 Server Adapter，使现有部署仍可运行。
- 所有 Data 单元测试、集成测试和 OpenAPI Contract Test 在根 Module 下可执行。

验收：根 Module 的 `go test ./...`、`go vet ./...`、Data 相关 race 测试通过；无 import internal 越界；Data API 行为未改变。

### M2：提取 Data Module Bootstrap 与 Application Port

Owner：Data

交付：
- 从 `cmd/market-data/main.go` 抽出可注入 Config、DB、Redis、Logger、HTTP Client、Clock 等依赖的 Module 构造函数。
- 返回 Data Application Port、HTTP Handler、Diagnostics、Start/Stop 或 Close 生命周期接口。
- 将 Provider 构造与能力注册从 main 中移入 Data Bootstrap；配置校验保持 Data Owner。
- 把 Data HTTP Handler 改为只依赖 Data Application Port，不直接拥有业务逻辑。
- 为内部调用定义强类型 Application DTO/Port；不要直接复用 HTTP JSON DTO。
- 现有独立 Data main 暂时改为调用同一 Module Bootstrap，避免出现两套 Provider 装配逻辑。

验收：Data 可以在单元测试中无 HTTP Server 启动；HTTP Handler 和 InProcess Adapter 使用同一 Data Application 实例；Start/Stop 可重复性与失败回滚有测试。

### M3：Root Server 同进程装配

Owner：Server + Data

交付：
- Server Bootstrap 创建共享基础设施并注入 Data Module。
- Analysis 的 MarketDataPort 改为 InProcess Adapter；不再使用 Base URL/API Key 做内部调用。
- Researchflow/Research 等模块如需 Data，统一依赖 Data Public Port；不得自行创建 HTTP Client 或绕过 Port。
- Data HTTP Adapter 由 Root Server Router 挂载到 `/market-api` 路由前缀，并由 Root Server 统一执行外部认证与授权。
- Request ID、Principal、Scope、purpose_code 通过可信 Context 显式传递。
- Data 的 Migration 与 Worker 由 Server 统一编排。

验收：同一 Data 请求不产生内部 HTTP 往返；不存在重复 Provider 实例、重复 DB Pool、重复 Migration、重复 Worker 或双重 Request ID。

### M4：WebUI 与部署切换

Owner：Server + WebUI

交付：
- Nginx 两个 API 前缀统一转发 Root Server。
- WebUI 删除 Data API Key 配置、localStorage 读写及请求 Header。
- 清理旧 localStorage 中 Data Key 字段；若历史上存放过真实有效 Key，部署前轮换旧 Key。
- Compose 删除独立 Data 服务及其健康依赖；Server 纳入 Data Provider 配置和 Data Worker。
- 更新 .env.example、部署说明、健康检查和回滚文档。

验收：浏览器 Network/Storage/Source Map/响应体中均无 Data API Key；`/market-api` 与 `/analysis-api` 均可用；Server 单容器重启后 Data 与 Analysis 同时恢复。

### M5：稳定性与拆分回归

Owner：Server + Data + Platform

交付：
- 启动失败回滚、优雅关闭、Worker drain、DB/Redis 断连恢复测试。
- Data Repository/Migration 只执行一次的集成测试。
- Provider 超时/失败、Data 质量降级、缓存不可用、HTTP 请求取消等回归。
- InProcess Adapter 与 Remote Adapter 的契约一致性测试。
- 记录未来恢复独立部署的触发阈值与拆分步骤。

验收：关键业务行为与迁移前一致；契约字段、单位、精度、PIT、授权和 Request ID 没有回退；回滚路径可执行。

---

## 9. 契约与文档同步要求

本方案获项目决策后，需要按以下方式同步，不允许各业务分支自行发明兼容语义：

| 契约/文档 | 调整内容 |
|---|---|
| SRV-CTR-001 | 补充 Data Module 注册、共享 DB/Redis、HTTP Router、Worker 生命周期、启动失败回滚和关闭顺序 |
| DATA-CTR-006 | 明确 X-API-Key 仅用于远程 Data HTTP Adapter；同进程内部 Port 不使用 Data API Key；保留外部 Adapter 的认证要求 |
| DATA-CTR-007 | Research 使用 Data Public Application Port 或其稳定 Remote Adapter；首批 DailyBarsV2 字段与 PIT 语义不变 |
| DATA-CTR-008 | 主体改由 Root Server 可信 Principal 注入；Manifest、Scope、purpose 和当前授权复核语义不变 |
| PLAT-CTR-005 | HTTP Client 只用于真实出站 Provider/外部依赖；Server→Data 内部调用不再经过 HTTP Client；Data Provider 网络安全仍必须遵守公共安全策略 |
| PLAT-CTR-006 | Server 统一创建共享连接池和 Migration Runner；Data 仍拥有自己的 Schema/Repository/Migration 内容 |
| PLAT-CTR-007 | Data Config 由 Server Bootstrap 装配，环境变量只在 Bootstrap 读取；模块 Config 保持独立类型 |
| 跨模块接入执行说明 | 从“Data 独立 Module 只能经 HTTP 接入 Platform”调整为“Data 与 Server 同一根 Module，通过公共 Port 接入；不 import 其他模块 internal” |
| Data 服务间认证映射与 Secret 注入设计 | 标注独立部署模式为兼容/未来模式；当前目标模式不需要 Server→Data API Key |
| 部署说明与 WebUI 验收清单 | 更新容器拓扑、Nginx 上游、API Key 清理、健康检查和回滚步骤 |

契约状态不能因文档变更自动升级为 frozen；仍需各 Owner 的 Schema/Golden/Contract Test 和 CI 证据。

---

## 10. 成本、收益与风险

### 10.1 一次性改造成本

基于当前已看到的结构估算；需要 M0 全量依赖审计后再收敛。

| 工作 | 估算 | 说明 |
|---|---:|---|
| M0 依赖/调用/迁移审计 | 1～2 人日 | 明确隐藏入口、配置和生命周期 |
| M1 Go Module 归并与依赖统一 | 1～2 人日 | import 路径、Go 版本、CI、Docker |
| M2 Data Bootstrap / Port 提取 | 2～3 人日 | Provider 装配较大，需抽离 main |
| M3 Server 同进程装配与资源复用 | 2～3 人日 | Data Application 注入、DB/Redis/Migration/Worker |
| M4 WebUI/Nginx/Compose 迁移 | 1～2 人日 | Key 清理、API 路由与部署 |
| M5 回归与部署验收 | 1～2 人日 | 行为一致性、安全、PIT、生命周期 |
| **合计** | **8～14 人日** | 不是承诺工期；不包含无关领域重构和新增业务功能 |

相比此前 6.5～12 人日估算，本次核对发现 Data Bootstrap 的 Provider capability 装配集中在独立 main，且数据库/Redis 生命周期需要合并治理，因此将估算上调至 8～14 人日。

### 10.2 长期收益

- 减少一个常驻容器、一套健康检查和一套服务间配置。
- Server 内部 Data 调用不再经过 TCP/JSON/API Key，减少网络跳数和重复序列化。
- 统一启动、日志、Request ID、诊断、资源池和 Worker 生命周期。
- 避免 Data 与 Server 对同一数据库各自维护连接池、迁移入口和部署依赖。
- 新模块默认采用同进程 Port，只有满足拆分条件才承担独立部署成本。

### 10.3 主要风险与控制

| 风险 | 控制 |
|---|---|
| Data 单个 Provider 阻塞影响 Server | Provider 超时、context 取消、并发限制、熔断和诊断隔离；不得把同步 Provider 调用放到启动关键路径 |
| Data 资源泄漏导致进程整体资源耗尽 | 统一连接池、并发预算、响应体限制、Worker 生命周期和资源关闭测试 |
| Data Handler 直接暴露绕过认证 | Root Server Router 统一认证；Data Handler 只作为内部 Adapter 挂载 |
| 一次性搬迁影响行情数据 | 不改 Schema/历史数据；先 Module 归并，再切换调用路径 |
| Go 依赖升级造成回归 | 固定版本、完整测试、依赖差异 Review；不做顺手升级 |
| 未来需要独立扩容 | 保留 Remote Adapter、HTTP DTO/Schema 和 Application Port；通过适配器替换恢复独立服务 |

---

## 11. 未来何时重新拆成独立服务

以下任一条件形成可量化、持续的实际需求时，再评估 Data 独立部署：

1. Data 需要独立扩容，CPU/内存/网络资源与 Server 明显不同。
2. Data Provider 任务的故障域或发布节奏需要与 Server 隔离。
3. 多个独立系统需要复用 Data API，且不能通过统一 Gateway/Server 入口解决。
4. Data 需要独立安全网络区、独立出站策略或独立凭据边界。
5. Data 需要独立部署窗口、版本节奏或 SLA。
6. 同进程资源竞争通过限流、Worker 隔离和资源预算仍无法解决。

重新拆分时：

- 保留 Data Application Port 作为领域边界。
- 将 InProcess Adapter 替换为 Remote HTTP Adapter。
- 使用现有版本化 DTO/OpenAPI、认证和 Request ID 契约。
- Data 重新获得独立 Bootstrap/进程入口，但不复制业务实现。
- 不改变 Data Repository Owner 和数据质量语义。

---

## 12. Review 清单与当前未决项

### 12.1 架构 Review

- [x] 目标运行时选择同进程模块化单体。
- [x] Server 保持唯一组合根。
- [x] Data 保持行情领域唯一 Owner。
- [x] 模块调用通过 Application Port，不访问对方 internal。
- [x] Data 的 HTTP API 保留为 Adapter，不作为内部调用必经路径。
- [x] DATA-CTR-008 授权语义不随 API Key 移除而取消。
- [x] MariaDB/Redis/Worker 生命周期归 Root Server 统一编排。
- [x] WebUI API 前缀可保留，Nginx 上游统一指向 Root Server。
- [x] 明确未来独立部署的触发条件。

### 12.2 实施前仍需核实

1. 根模块当前 `mysqlredis.Store` 与 Data `repository.Store` 的实际 DB/Redis 资源是否可抽象为共享句柄；确认是否存在相互依赖或不同 DSN/事务设置。
2. Data 所有 Migration 的执行顺序、namespace 和幂等性；当前 Data main 与 Server storage Open 是否都执行迁移。
3. Data 真实 HTTP API 是否还有除 WebUI、Analysis Adapter 以外的调用者。
4. Compose 使用的 `go-core` 历史入口是否仍在部署/运维脚本中；不能只按当前 docker-compose 判断所有环境。
5. WebUI 当前 Data Key 输入与 localStorage 使用点、Owner Token/session 认证方式及 CORS/CSRF 行为。
6. Data HTTP API 是否必须继续允许独立部署调用；如保留，需定义独立模式的 Key 生命周期与网络 ACL。
7. Data 的 Redis Cache 是否可以共享根 Server Redis Client，还是因 Redis DB、超时、命名空间需求保留独立 Client（共享底层连接池与独立 Client wrapper 二选一由审计决定）。

### 12.3 明确不在本轮做的事情

- 不在 `feat/design_doc` 修改业务 Go 代码。
- 不直接修改 `feat/full-go-data-providers-api-switch`、`feat/platform-foundation`、WebUI 或其他模块分支。
- 不删除 DATA-CTR-006、PLAT-CTR-005 或 Data HTTP API 的既有定义；先按本方案明确适用模式，再由项目总规划更新契约登记。
- 不因部署合并把所有业务模块强行放入同一个 package。
- 不创建 Portfolio、Alert、Report 等候选模块分支。
- 不以本次设计 Review 代替模块 CI、集成测试或真实部署验收。

---

## 13. 建议执行顺序

1. 在项目总规划 Review 本方案，并确认 DATA-CTR-006 远程/进程内双模式边界。
2. 先执行 M0，只读完成依赖、数据库、Redis、Worker、WebUI 路由和 go-core 入口审计。
3. M0 通过后，分配 M1/M2 给 Data Owner，M3 给 Server Owner；两者可并行，但必须以相同设计基线协作。
4. M1/M2 合并到集成基线并通过模块 CI 后，再进行 M3 同进程装配。
5. M3 通过后，WebUI/Server 执行 M4；最后完成 M5 部署验收。
6. 所有契约继续保持 `pending_evidence`，直到真实测试和 CI 证据齐全。

**最终决议：采用方案 B。先统一 Module 和进程，再逐步切换内部 Port 与部署；不进行无边界的大规模代码搬迁。**
