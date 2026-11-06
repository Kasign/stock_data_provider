# Market Data 模块化单体迁移计划与进度

- 维护分支：`feat/full-go-data-providers-api-switch`
- 迁移决策：采用 Root Server 同进程模块化单体
- 方案依据：`doc/整体任务规划/Market Data并入Root Server模块化单体调整方案.md`
- 总体规划：`doc/整体任务规划/整体任务规划总览.md`
- 跨模块准入：`doc/整体任务规划/跨模块接入执行说明.md`、`doc/整体任务规划/跨模块接入准入清单.md`
- 本记录建立时的代码基线：`d8595f408337f62be3697aac661ba1570fdd3667`
- 当前实施状态：`not_started`（已建立执行计划；尚未执行代码迁移）

> 本文记录 Data 从独立 Go Module/服务迁入 Root Server 根 Go Module、同进程运行的实施计划和进度。若本文与上述架构决策文档冲突，以架构决策文档为准；公共平台及 DATA 契约以冻结契约文档为准。本文不重新定义契约。

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

状态：`pending`

工作项：

- [ ] 获取迁移执行时最新 HEAD、分支状态和工作区/远端变更基线。
- [ ] 盘点根 Module、Data Module 的 Go 版本、依赖、replace、生成代码和工具链。
- [ ] 盘点 Data 全部 package、命令入口、测试、testdata、配置文件、脚本和 Docker 构建上下文。
- [ ] 盘点 Root Server 到 Data 的所有调用点，标记请求/响应 DTO、认证和超时行为。
- [ ] 盘点 Data HTTP 路由、中间件、OpenAPI、WebUI/Nginx 路由依赖。
- [ ] 盘点 MariaDB Pool、Redis Client、Migration、Probe 持久化和 Worker 的创建/关闭路径。
- [ ] 盘点 Compose、Dockerfile、CI workflow、部署脚本及 go-migration profile。
- [ ] 为每项迁移对象记录源路径、目标路径、依赖、风险、测试和回滚方式。
- [ ] 标记迁移阻断项；没有明确处理策略的阻断项不得进入对应阶段。

验收：

- [ ] 有完整迁移对象清单，覆盖生产代码、测试、配置、构建和部署文件。
- [ ] 所有跨 Module import 与内部包边界有明确处理方式。
- [ ] 资源所有权和生命周期有明确图示/清单。
- [ ] 迁移前基线测试和 CI 状态可追溯。

### M1：Go Module 归并

状态：`pending`

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

状态：`pending`

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

### M3：Root Server 同进程装配

状态：`pending`

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
| M0 迁移前审计 | pending | 已定位主要目录，需逐项核对依赖、调用点、资源和 CI |
| M1 Go Module 归并 | pending | 未开始 |
| M2 Application Port/Bootstrap | pending | 未开始 |
| M3 Root Server 装配 | pending | 未开始 |
| M4 共享资源统一 | pending | 未开始 |
| M5 部署/WebUI 切换 | pending | 未开始 |
| M6 迁移验收 | pending | 未开始 |
| 公共平台契约接入 | blocked_by_migration | 迁移完成后按冻结契约执行 |

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

## 7. 开始实施前检查

进入 M0/M1 前必须重新读取当前分支 HEAD 和方案文档。本文创建时的 SHA 仅是记录基线，不是后续写入锁。任何提交都必须以当时最新 HEAD 创建 Tree/Commit，并使用 `expected_sha` 更新分支；发现并发提交时停止写入并重新审计。
