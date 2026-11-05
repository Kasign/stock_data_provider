# 新闻模块专项文档

> 状态：N1-A 业务架构收敛中；DB、Redis、通用诊断统一延期至公共服务阶段  
> 开发分支：`feat/research-news-ingestion`  
> 基线分支：`feat/full-go-data-providers-api-switch`

## 目标

建设 A 股低频研究助手的内部新闻业务能力：来源接入、内容采集、标准化、去重、版本管理、证券关联和内部查询能力。通用 DB/Redis 接入与诊断能力由后续公共服务统一提供；当前新闻模块不重复建设。当前不建设新闻独立对外 HTTP API。

本专项采用模块化单体：新闻业务模块位于根 Go 模块，由 `stock-analysis-server:8092` 统一组装和部署；不新增独立微服务、端口、部署入口或新闻专属 HTTP 路由。新闻能力通过同进程 Application 接口供分析流程和其他内部调用方使用。若现有 Server API 将来需要向外提供相关能力，只能由该既有 API 统一承载并沿用其安全边界。

## 文档索引

| 文档 | 内容 |
|---|---|
| [新闻模块总体设计](新闻模块总体设计.md) | 边界、架构、依赖和实施原则 |
| [新闻数据模型与质量契约](新闻数据模型与质量契约.md) | 时间语义、数据实体、版本、去重和质量 |
| [新闻Provider接入与诊断规范](新闻Provider接入与诊断规范.md) | Provider 适配与业务质量边界 |
| [新闻来源授权矩阵](新闻来源授权矩阵.md) | Provider 输出权利、原始来源抓取和内容留存约束 |
| [新闻API契约](新闻API契约.md) | 内部能力边界及未来 HTTP API 评估草案（当前不对外提供） |
| [openapi.yaml](openapi.yaml) | 暂缓启用的未来 HTTP API 草案，不得据此注册路由 |
| [新闻模块开发任务与验收清单](新闻模块开发任务与验收清单.md) | 阶段、任务和验收门槛 |
| [Python新闻能力迁移清单](Python新闻能力迁移清单.md) | 旧调用盘点和迁移退役条件 |
| [新闻模块集成与回归方案](新闻模块集成与回归方案.md) | 分支、数据库、兼容和回归约束 |

## 当前执行状态

- [x] 创建独立开发分支
- [x] 建立专项设计文档骨架
- [ ] N1-A：完成新闻业务边界、来源授权、证券关联、Python 迁移、共享 Provider 适配和 Application 契约评审
- [ ] N2：实现独立领域模型与现有搜索 Provider 适配
- [ ] N3：实现来源采集、正文解析和标准化
- [ ] N4：实现身份识别、去重、版本和证券关联业务规则
- [ ] N5：实现旧 Python 能力兼容适配与切换准备
- [ ] N6：公共服务就绪后接入 DB/Redis/通用诊断并完成持久化
- [ ] 用户评审 N1-A 后进入 N2

## 重要说明

本文档中的目标架构和契约是设计基线，不代表对应代码已经实现。所有“现有能力”必须以 N1 对当前分支代码的核查结果为准。未经核实不得把历史对话中的代码状态当作当前事实。

本专项开发期间只修改新闻专项代码和 `doc/news/**`。公共数据侧规划、公共 API 清单、质量契约、Python Bridge OpenAPI、WebUI 和整体规划文档在集成阶段再按实际验收结果更新。

## N1 盘点进度（2026-11-05）

已完成第一轮静态代码盘点，部署归属已确认：新闻作为根 Go 模块内部业务模块随 `stock-analysis-server:8092` 部署。当前没有新闻独立对外 API 或独立服务计划；HTTP 路由不属于本阶段交付。详见《新闻模块总体设计》N1 章节。

N1 已追加核对固定旧 Python 上游的三类新闻/情报持久化对象及旧 intelligence API、Bot /research 入口，并从整体规划分支核对 DATA-016、MIG-005、MIG-006、MIG-008、RESEARCH-004 的原始任务边界。最终架构仍未冻结，详见《新闻模块总体设计》9.11 及任务映射章节。

### N1 第二轮核验补充（2026-11-05）

已进一步核对旧 Python 搜索/情报调用方、证券主数据证据和来源安全边界。结论：旧链路同时由 API、Bot/Agent、个股分析和大盘复盘消费；来源 URL 有 SSRF 防护，但没有逐来源授权/robots/内容保留策略元数据。当前仍不开始业务代码开发。

## N1 已确认的架构决策（2026-11-05）

- **部署位置：** 根目录统一部署的 `stock-analysis-server:8092`。
- **模块边界：** 新闻业务模块位于根 Go 模块（建议落点 `internal/news/`），由 `cmd/analysis-api` 组装；不放入 `go-core` 或 `services/market-data`。
- **内部调用：** 同进程模块通过 Application 接口直接调用，不经过内部 HTTP，不设置新闻专属 Service Token。
- **HTTP 边界：** 当前不注册新闻专属 HTTP 路由，不独立对外提供新闻查询、搜索、来源管理或诊断 API。
- **内部调用：** 新闻 Application 由同进程分析流程或其他内部调用方直接使用；不经过内部 HTTP，不设置新闻专属认证。
- **既有 Server API：** 若未来某个既有 Server API 需要消费新闻能力，由该 API 自身负责认证、授权、限流和日志；新闻模块不绕过该边界。
- **运维管理：** 来源配置、主动采集、外部 URL 测试和授权策略修改暂作为内部运维能力，不新增公网管理路由。
- **阶段边界：** 本决策只冻结部署归属和认证责任边界，不代表 Server 认证已经实现，也不代表新闻 API、业务代码或数据库已实现。N1 其余未决项仍需完成评审。


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