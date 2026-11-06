# 新闻模块专项文档

> 状态：N1-A 已评审确认；N2 根模块验证通过；N3 授权、离线解析、候选标准化、内容分层和业务质量评估首批完成；在线抓取仍等待平台安全门禁  
> 开发分支：`feat/research-news-ingestion`  
> 基线分支：`feat/full-go-data-providers-api-switch`

## 目标

建设 A 股低频研究助手的内部新闻业务能力：来源接入、内容采集、标准化、去重、版本管理、证券关联和内部查询能力。通用 DB/Redis 连接生命周期、Migration 执行治理、诊断执行框架等由公共平台提供；新闻模块仍拥有自己的 Repository、SQL、业务表和质量规则。当前不自行创建连接池或通用诊断框架。当前不建设新闻独立对外 HTTP API。

本专项采用模块化单体：新闻业务模块位于根 Go 模块，由 `stock-analysis-server:8092` 统一组装和部署；不新增独立微服务、端口、部署入口或新闻专属 HTTP 路由。新闻能力通过同进程 Application 接口供分析流程和其他内部调用方使用。若现有 Server API 将来需要向外提供相关能力，只能由该既有 API 统一承载并沿用其安全边界。

## 文档索引

| 文档 | 内容 |
|---|---|
| [N1 当前 HEAD 现状盘点与设计评审报告](N1现状盘点与设计评审报告.md) | 2026-11-06 平台合并后最新代码证据、差距、风险与评审结论 |
| [N1-A 新闻基础设施架构评审报告](N1-A新闻基础设施架构评审报告.md) | 当前最终架构、领域契约、风险和用户确认项 |
| [新闻模块与公共平台接入矩阵](新闻模块与公共平台接入矩阵.md) | PLAT 契约映射、依赖门禁、所有权和跨分支协作 |
| [N1-A 公共平台规划对齐补充评审](N1-A公共平台规划对齐补充评审.md) | 最新平台分支规划核验、任务编号纠偏和新闻实施门禁 |
| [新闻模块总体设计](新闻模块总体设计.md) | 边界、架构、依赖和实施原则 |
| [新闻数据模型与质量契约](新闻数据模型与质量契约.md) | 时间语义、数据实体、版本、去重和质量 |
| [新闻Provider接入与诊断规范](新闻Provider接入与诊断规范.md) | Provider 适配与业务质量边界 |
| [新闻来源授权矩阵](新闻来源授权矩阵.md) | Provider 输出权利、原始来源抓取和内容留存约束 |
| [新闻API契约](新闻API契约.md) | 内部能力边界及未来 HTTP API 评估草案（当前不对外提供） |
| [openapi.yaml](openapi.yaml) | 暂缓启用的未来 HTTP API 草案，不得据此注册路由 |
| [新闻模块开发任务与验收清单](新闻模块开发任务与验收清单.md) | 阶段、任务和验收门槛 |
| [旧 Python 能力盘点与历史数据评估](Python新闻能力迁移清单.md) | 旧调用链和历史数据语义参考，不构成兼容开发任务 |
| [新闻模块集成与回归方案](新闻模块集成与回归方案.md) | 分支边界、公共服务依赖和根模块回归约束 |

## 当前执行状态

- [x] 创建独立开发分支
- [x] 建立专项设计文档骨架
- [x] N1-A：完成新闻领域模型、时间/身份/版本/关联证据、Provider 与 Application 契约及跨文档内部评审
- [x] N2：Domain、Application Port、Bocha/Tavily/Brave/SearXNG Adapter 与 fake 测试已提交；根模块 gofmt、test、race、vet 和分析服务镜像构建通过
- [x] N3 首批：Provider/Source/Endpoint 授权交集评估、用途/保留期限限制、内容类型与存储模式一致性、HTTP(S) URL 规范化与精确主机/端口 allowlist；固定测试通过
- [x] N3 离线业务质量：按元数据、内容、来源身份三个维度评估 valid/degraded/invalid/unknown；不把 Provider 摘要提升为来源正文，不把发布者文本视为已解析来源
- [ ] N3 在线采集：Feed 解析与 Endpoint 传输分派已有代码；平台 PLAT-008 BoundDoer 已随平台分支合并进入当前基线并适配 News HTTPDoer；生产逐 endpoint 注入、来源授权/出口验收未完成，PLAT-002 仍未冻结
- [x] N4 Domain/Application 首批：身份识别、Observation 幂等语义、内容版本、证券/行业关系和转载关系规则已有实现与离线测试
- [ ] N4/N6 后续：真实采集编排、生产 Repository、数据库唯一约束/事务并发与历史版本持久化仍未完成
- [ ] N5：按公共契约接入 Config/Secret、Logger/Fault/Request ID、HTTP Client；NEWS-CTR-002 Binding DTO/Validator 已实现结构绑定和 nullable 字段显式 null/缺失区分；fingerprint JCS 复核等待公共 GOV-CTR-006 Verifier，尚未实现授权签发、当前策略读取时复核和授权竞态保护
- [ ] N6：PLAT-009 Storage/Migration 原语已合并并通过 MariaDB/Redis CI 集成；News 自有 Repository/SQL/Migration 与根组合根注册仍待实现；Redis 按需
- [ ] N7：接入公共 Diagnostics；Task/Audit/Metrics 按真实需求评估
- [ ] N8：完成 News 与公共平台集成验收
- [x] N1-A 用户评审确认；N2 已完成并进入 N3

## 公共平台协作说明

公共平台分支 `feat/platform-foundation` 本轮只读核对 HEAD `55cbd0d642ac2a9b37e0ebfb8e7be0034af87f5e`：PLAT-008 `httpclient.BoundDoer` 已实现并有平台专项 CI 通过记录；PLAT-002 仍标记待评审，PLAT-013 仍在设计/领域接入阶段，PLAT-010 已收敛为领域边界评审而非通用队列。当前分支已合并平台 `BoundDoer` 与 `storage` 实现；`BoundDoer` 与 News `HTTPDoer` 兼容，Storage 提供 SQL/Redis 生命周期及 Migration Runner。News 仍缺生产逐 endpoint 注入、News Repository/Migration、根组合根共享资源与 Migration 注册；PLAT-002 仍未冻结。平台分支只读跟踪，不在本分支修改。生产 DNS/出口、真实 Provider 凭证/配额、Source/Endpoint 授权仍是独立验收门禁。Metrics/Audit 和 Redis 不作为新闻首期门禁。

## 重要说明

本文档中的目标架构和契约是设计基线，不代表对应代码已经实现。所有“现有能力”必须以 N1 对当前分支代码的核查结果为准。未经核实不得把历史对话中的代码状态当作当前事实。

本专项开发期间只修改新闻专项代码和 `doc/news/**`。公共数据侧规划、公共 API 清单、质量契约、Python Bridge OpenAPI、WebUI 和整体规划文档在集成阶段再按实际验收结果更新。

## N1 盘点进度（2026-11-05）

已完成第一轮静态代码盘点，部署归属已确认：新闻作为根 Go 模块内部业务模块随 `stock-analysis-server:8092` 部署。当前没有新闻独立对外 API 或独立服务计划；HTTP 路由不属于本阶段交付。详见《新闻模块总体设计》N1 章节。

N1 已完成固定旧 Python 上游的三类新闻/情报持久化对象、旧 intelligence API、Bot /research 入口及整体规划任务边界核对。N1-A 架构评审已获确认，详见《N1-A新闻基础设施架构评审报告》。

### N1-A 最终收口（2026-11-05）

已完成旧 Python 主要调用链和历史表语义盘点、证券主数据核验、Provider/来源公开条款首轮核查、领域模型与 Application 契约设计。N1-A 已评审确认，N2 根模块验证已通过；N3 已实现授权交集、URL 离线校验、RSS/Atom/HTML 离线解析、候选标准化和内容分层。平台分支当前已有 PLAT-008 BoundDoer 及专项 CI，但 News 分支尚未包含该实现，PLAT-002 也未冻结；在线抓取仍未开放。N4 Domain/Application 首批能力已有离线测试，生产编排与 Repository 仍待完成。未获明确许可的来源保持 pending/restricted。

## N1 已确认的架构决策（2026-11-05）

- **部署位置：** 根目录统一部署的 `stock-analysis-server:8092`。
- **模块边界：** 新闻业务模块位于根 Go 模块（建议落点 `internal/news/`），由 `cmd/analysis-api` 组装；不放入 `go-core` 或 `services/market-data`。
- **内部调用：** 同进程模块通过 Application 接口直接调用，不经过内部 HTTP，不设置新闻专属 Service Token。
- **HTTP 边界：** 当前不注册新闻专属 HTTP 路由，不独立对外提供新闻查询、搜索、来源管理或诊断 API。
- **内部调用：** 新闻 Application 由同进程分析流程或其他内部调用方直接使用；不经过内部 HTTP，不设置新闻专属认证。
- **既有 Server API：** 若未来某个既有 Server API 需要消费新闻能力，由该 API 自身负责认证、授权、限流和日志；新闻模块不绕过该边界。
- **运维管理：** 来源配置、主动采集、外部 URL 测试和授权策略修改暂作为内部运维能力，不新增公网管理路由。
- **阶段边界：** 本决策只冻结部署归属和认证责任边界，不代表 Server 认证已经实现，也不代表新闻 API、业务代码或数据库已实现。N1-A 领域架构与平台接入边界已完成内部收敛，最终以《N1-A新闻基础设施架构评审报告》供用户确认。


## 2026-11-05 阶段调整

用户已确认：新闻模块不再自行建设 DB/Redis 接入和通用诊断能力，统一等待公共服务提供。新闻当前阶段只建设不依赖这些基础设施的业务能力；质量规则仍属于新闻领域，不能与通用诊断混为一谈。详细边界和阶段顺序以《新闻模块总体设计》9.16 及《新闻模块开发任务与验收清单》为准。


## 2026-11-05 架构决策更新：根模块原生实现，不建设共享 Provider Module

用户已明确：整体服务尚未投入使用，不要求兼容旧 Go/Python 新闻调用链。本专项按最终目标架构直接实现，不为未上线旧服务保留适配层、fallback 或双轨运行。

- 新闻 Provider 实现直接归属根 Go 模块 `internal/news/adapters/provider/`，不创建 `shared/newsprovider` 独立 Go Module。
- 根模块是唯一新闻业务运行时；新闻业务 Domain/Application/Provider Adapter 在同一 Go Module 内组织，依赖方向由包边界和接口控制。
- 现有 `go-core/internal/adapters/newssearch` 仅作为协议实现和边界行为的参考来源；按新领域契约迁移/重构必要 Provider 能力，不复制旧 Searcher 聚合、缓存、Python fallback、旧 DTO 或旧业务接口。
- 不保留旧 `NewsSearcher` 兼容 Adapter，不接入 Python Bridge fallback，不为 go-core 维护第二套新闻 Provider 调用链。确认没有其他有效调用后，清理本专项范围内废弃的旧 Go 新闻搜索集成；不擅自删除仓库中与新闻无关的 go-core 模块。
- Provider 结果必须从最初适配层开始保留 provider_code、provider_item_id（若有）、publisher/source、原始 URL、原始发布时间文本、observed_at、query dimension 和字段缺失信息；不先映射成旧 NewsItem 再补信息。
- 搜索 Provider、官方公告源和原始发布来源是不同概念，公告源采用独立端口，不伪装为搜索 Provider。
- 不设置混合结果跨请求缓存。首期 Provider Adapter 不缓存；缓存若未来必要，必须由明确的 Provider 留存策略和业务收益驱动。
- 不为旧 go-core Docker 构建上下文调整仓库布局；根模块按自身构建上下文部署。独立共享模块与本地 replace 均不再是方案的一部分。
- Python 旧 API、Agent、Bot、旧 NewsItem DTO 兼容和旧库双写/在线迁移不属于本专项交付。历史数据若未来确有价值，另行按公共存储迁移流程评估，不反向约束新业务模型。

本决策优先于本文此前关于共享 Provider Library、go-core 双调用方适配、Python fallback 和旧 DTO 兼容的建议。

## 2026-11-05 实施策略更新

整体服务尚未投入使用，新闻模块直接按根 Go 模块最终架构建设，不承担旧 Go/Python 服务兼容。旧 Python 文档仅作为历史调用链和数据语义盘点，不再代表兼容迁移任务。当前以《新闻模块总体设计》9.21、《新闻模块开发任务与验收清单》第十一节为准。


## NEWS-CTR-002 授权绑定校验进度（2026-11-06）

- 已实现强类型 Binding / AuthorizationRef / PolicySnapshot DTO。
- Validator 校验可信 caller、资源、用途、内容级别、策略版本引用、策略 tuple 唯一性和排序。
- `provider_ref`、`endpoint_ref`、`expires_at` 均区分字段缺失与显式 `null`；Schema 要求字段存在，null 按各字段契约表达不适用/无固定过期时间。
- Fingerprint 当前只校验小写 64 位 SHA-256 hex；JCS + SHA-256 复核等待公共 GOV-CTR-006 Verifier 进入共同基线，不在 News 内复制算法。
- 本 Validator 不签发授权、不读取当前策略、不提供受限内容，也不替代读取时 Owner 复核。
- NEWS-CTR-002 仍为 pending_evidence，不标记 L2/frozen。


## NEWS 授权用途与内容级别映射（2026-11-06）

- `internal_personal_research` 明确映射为 `research_analysis`；`public_display` 明确映射为 `user_display`。
- `internal_business` 不能自动推断为 `research_analysis` 或 `risk_review`；`redistribution`、`model_training` 不属于 NEWS-CTR-002 当前用途枚举，均拒绝隐式映射。
- `ContentMetadata` / `ContentTitle` → `metadata`；`ContentAbstract` → `summary`；`ContentSnippet` → `snippet`；`ContentFullText` → `full`。
- `ContentAttachment` 与未知内容类型不自动映射到 `full`，必须单独确认内容权限后才能处理。
- `risk_review` 是显式授权用途，不从现有通用 `Purpose` 值推断。


## NEWS-CTR-002 授权内容放行门禁（2026-11-06）

新增 `EvaluateNewsAuthorizationUse`，对已提供的 AuthorizationRef 执行本地 fail-closed 判定：

- 仅显式 `allowed` 且授权内容级别覆盖请求级别时允许披露。
- `conditional` 在授权 Owner 完成条件评估前一律拒绝；Conditions 非空不等于条件已满足。
- `denied`、`unknown`、未生效、过期、无效时钟和 Binding 不一致均拒绝。
- 过期边界为 `now >= expires_at` 即拒绝。
- 该门禁不刷新当前策略目录、不复算 GOV-CTR-006 JCS 指纹、不提供撤销实时性，也不保护数据库读取竞态。生产读取仍需先由 News Owner 复核当前策略，再在同一一致性边界内读取内容。


## CollectSource 内容级别裁剪（2026-11-06）

Review 发现 CollectSource 原先只校验请求 ContentKind，却将 FeedEntry 的全部 ContentLayers 原样返回，存在请求 metadata 却连带返回来源摘要/正文/附件元数据的越权披露风险。现已在 Application 返回边界调用 `domain.RestrictContentLayers`：

- metadata/title 不返回来源摘要、正文、外部正文引用或附件元数据。
- abstract 只保留来源摘要；snippet/full 仅保留明确分类且不高于请求级别的正文载荷。
- parser 未确认 Kind 的正文保持 unknown，不因调用方请求 full_text 就提升为全文。
- attachment 请求只返回附件元数据，不返回文章摘要或正文；附件仍不下载。
- ProviderSummary 与来源授权内容分层，不被提升成来源摘要或正文。

该裁剪是返回数据最小化，不替代 Provider/Source/Endpoint 当前策略复核，也不代表正文持久化已获授权。


### NEWS-CTR-002 策略快照完整性补充（2026-11-06）

Binding 中 `provider_ref` / `endpoint_ref` 非 null 时，PolicySnapshot 必须分别包含对应的 `provider_output` / `endpoint` 实际生效版本；显式 null 才表示该层不参与本次决策（Endpoint 可继承 Source）。缺失任一非 null 引用对应的 tuple 必须拒绝。


## 2026-11-06 平台合并后 N1 当前 HEAD 复核

当前盘点基线：`aca8ff6b6bd2a03adf5ded075f354a77f6dc3370`。完整代码证据、旧 Python 调用链、平台合并变化、风险和设计评审见《[N1 新闻模块现状盘点与设计评审报告](N1现状盘点与设计评审报告.md)》。

- 平台 `httpclient.BoundDoer` 已进入当前分支，接口与 News `app.HTTPDoer` 兼容；News 生产组合根尚未逐 Provider/Source endpoint 注入。
- 平台 `storage` 已进入当前分支，MariaDB/Redis CI 集成与 Race 通过；根 Server 仍使用旧 mysqlredis Store，News Repository/Migration/注册尚未完成。
- Diagnostics/Config/Security 等平台原语已合并；News 领域 Check、Bootstrap 和服务路由仍未接入。
- 最新身份修正拒绝候选及已存凭据 Hash 不一致；最新授权修正拒绝 `expires_at == decided_at` 的零有效期授权。
- PLAT-002 仍待冻结，平台 L1 证据不等于 News A2/A3 接入通过。
- N1 盘点完成，等待用户确认；不自动进入后续业务开发。
