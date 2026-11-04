# Python 能力盘点与 Go 迁移映射

- 所属规划：A 股研究助手整体任务规划
- 所属分支：feat/design_doc
- 盘点日期：2026-11-04
- 任务编号：MIG-001（阶段性静态盘点）
- 当前状态：**完成：否**
- 对应总规划：整体任务规划总览.md v1.1

> 本文是基于 GitHub 当前分支和固定 Python 上游源码的静态盘点，不是生产运行时清点，也不代表任何 Python 能力已经迁移完成。进程、容器、真实环境变量、生产调用量、实际数据库和 Redis Key 等仍需在目标环境只读补证。

---

## 1. 核验基线与范围

| 仓库 / 分支 | 核验 SHA | 用途 |
|---|---|---|
| Kasign/stock_data_provider / feat/design_doc | 00b4a733001d5b1eb3ef539b7e39a3c43749bb27 | 总规划基线 |
| Kasign/stock_data_provider / feat/full-go-data-providers-api-switch | d91b2abf1fbc2a55e5db07ac06097a42e3436342 | Go 服务端、Bridge、部署和 CI 基线 |
| Kasign/daily_stock_analysis / main 固定提交 | 7a21cd76952c7fe44a7fc5a6113a5b2b8a16bb32 | 旧 Python 主项目源码基线 |

数据侧分支完整 Git Tree 共 447 个路径。本轮只通过 GitHub 接口读取分支、Tree 和文件；未 clone，未修改数据侧、WebUI 分支、代码、测试、部署文件或 PR。

固定旧 Python 上游提交 Git Tree 共 1,217 个路径，其中 656 个 Python 文件。主要目录：

| 上游目录 | 可见职责 | Go 目标模块 |
|---|---|---|
| api/ | FastAPI 应用、鉴权、中间件、REST 路由和 Schema | API / Transport |
| data_provider/ | Efinance、AkShare、Tushare、Pytdx、BaoStock、腾讯、TickFlow、长桥、富途等 | Provider Adapters / Market Data / Fundamentals |
| src/agent/、src/llm/ | Agent、工具、技能、模型后端、路由、上下文、生成和用量 | Research Orchestrator / LLM Gateway |
| src/services/、src/core/ | 分析、筛选、回测、告警、组合、历史、任务队列、调度、市场复盘 | Application / Quant / Backtest / Persistence |
| src/repositories/、src/storage.py | SQLite ORM、Repository 和历史持久化 | Persistence |
| src/notification* | 通知规则、路由、诊断和多渠道发送 | Notification |
| src/search_service.py、src/market_analyzer.py | 新闻检索、市场概览和复盘 | Research Ingestion / News / Event |
| bot/ | 命令、消息路由、钉钉、飞书、Discord 等 | API / Transport / Notification |
| main.py、server.py、webui.py | CLI、服务启动、旧 Web/API 入口 | Bootstrap / API；旧 UI 接管范围需确认 |
| scripts/、tests/、evals/ | 运维脚本、测试和 Agent 评估 | Go 测试/验收工具 |
| apps/、静态资源和模板 | 前端、桌面/打包资源、模板 | 不等同于 Python 业务运行时，逐项确认保留方式 |

全面 Go 原生化指生产业务服务、后台任务、调度、数据访问、模型适配、通知和业务 API 迁移；不意味着把前端静态资源改写成 Go，也不要求 Go 自己实现模型推理。外部/本地模型服务可以继续提供推理。

---

## 2. 当前仓库中的 Python 运行时和调用链

### 2.1 当前仓库可见文件

| 路径 | 职责 | 盘点结论 |
|---|---|---|
| python_bridge/capability_app.py | FastAPI 内部服务，8090 端口，提供健康检查和内部 POST 路由 | Python 服务代码仍存在 |
| python_bridge/capabilities/market_data.py | 调用旧 DataFetcherManager 获取日 K、实时行情和辅助数据 | 行情/辅助兼容层 |
| python_bridge/capabilities/search.py | 调用旧 src.search_service、src.market_analyzer | Python 新闻搜索仍存在 |
| python_bridge/capabilities/llm.py | 调用旧 src.analyzer、src.market_analyzer，执行旧结果后处理 | Python LLM 分析仍存在 |
| python_bridge/capabilities/persistence.py | 调用旧 src.storage、src.repositories.analysis_repo 和市场复盘持久化 | Python 仍拥有业务写入 |
| python_bridge/capabilities/notification.py | 调用旧 src.notification | Python 通知发送仍存在 |
| python_bridge/capabilities/common.py | JSON、数值、行情、日 K、新闻和结果对象转换 | 兼容转换逻辑 |
| python_bridge/go_core_client.py | 供旧 Python API/CLI/Bot/Scheduler 调用 Go Core；GO_CORE_ENABLED 显式启用 | Python → Go 迁移入口 |
| python_bridge/tests/ | Bridge 转换、鉴权、客户端和旧后处理测试 | Python 测试仍存在 |
| scripts/compare_market_data_exports.py 等 | Python/Go 样例对比工具 | Python 验收脚本 |
| scripts/verify-go-migration.sh | Go 测试外还执行 Python compileall、pytest | 迁移脚本仍依赖 Python |
| .github/workflows/data-quality-parity.yml | 安装 Python 3.12，执行对比工具测试和兼容标记检查 | Python 仍进入 CI |
| cmd/pythonapi-smoke/ | Go 调用旧 Python API 的 quote/history/profile/overview | 旧 API 调用入口 |
| internal/adapters/pythonapi/ | 旧 Python HTTP API Client、Collector、Config、DTO | 旧 API Adapter 仍存在 |
| go-core/internal/adapters/pythonbridge/ | Go Core 调用 Python Bridge 的 Client 和业务 Adapter | Go → Python 生产依赖仍存在 |
| python_bridge/Dockerfile | Python 3.12 镜像，启动 Uvicorn 8090 | Python 镜像配置仍存在 |
| docker/docker-compose.go-migration.yml | 可选 profile 启动 Python Bridge、Go Core、Go 行情服务 | 迁移拓扑仍包含 Python 容器 |

当前数据侧 Git Tree 未发现旧 Python 主项目的 src/、data_provider/、api/、bot/ 源码目录，也未发现根目录 requirements.txt。但 Bridge 明确导入 src.*，其 Dockerfile 执行 COPY requirements.txt。Bridge 依赖仓库外的旧源码/依赖上下文，当前仓库快照不能独立证明该镜像可构建。

### 2.2 Go Core → Python Bridge

当前 go-core/cmd/analysisd/main.go 强制校验 PYTHON_BRIDGE_TOKEN，创建 Python Bridge Client，并在 ReadyCheck 中探测 Python Bridge 和 Go 行情服务。它装配：
- Go 原生行情 Client；
- Go 原生新闻搜索（仍传入 Python SearchAdapter 作为 fallback）；
- Python AnalyzerAdapter；
- Python PersistenceAdapter；
- Python NotificationAdapter。

Go Core 当前仍将 Python Bridge 作为启动就绪的必需依赖。调用拓扑：

    Go Core /analysisd
      ├── Go Market Data：个股 Quote / DailyBar / 部分市场概览
      ├── Go Native News Search：配置的远程新闻来源
      │     └── 无可用结果时仍可能回退 Python SearchAdapter
      └── Python Bridge（必需）
            ├── stock-auxiliary：筹码、财务辅助上下文、所属板块
            ├── market-overview：部分市场概览回退
            ├── stock-news / market-news
            ├── llm/analyze-stock / generate-market-review
            ├── persistence/daily-bars / stock-analysis / market-review
            └── notification/stock-analysis / market-review

具体判断：
1. 个股实时行情和日 K 已由 Go 行情服务提供，Go Core 不允许这两项回退 Python。
2. 筹码、财务辅助和所属板块仍由 Python stock-auxiliary 提供。
3. Go 原生新闻搜索已存在，但 newssearch.NewFromEnv(legacySearch, ...) 仍保留 Python fallback。
4. LLM 分析和市场复盘生成仍通过 Python AnalyzerAdapter。
5. 日 K、分析历史和市场复盘持久化仍通过 Python PersistenceAdapter。
6. 通知仍通过 Python NotificationAdapter。
7. Go Core ReadyCheck 对 Python Bridge 的健康状态有硬依赖。

### 2.3 Python → Go Core

python_bridge/go_core_client.py 是供旧 Python API/CLI/Bot/Scheduler 使用的统一 Go Core Client，GO_CORE_ENABLED 默认关闭，显式启用才切换。当前仓库包含客户端工具，但不包含全部旧调用方源码；必须继续从固定上游源码和目标环境确认实际调用点、开关值、生产流量及旧路径是否仍被调用。

### 2.4 另一个旧 Python API 适配链

除 Python Bridge 外，当前仓库还有 internal/adapters/pythonapi/，配置项包括 PYTHON_API_BASE_URL、PYTHON_API_TIMEOUT、PYTHON_API_HISTORY_DAYS、PYTHON_API_MAX_CONCURRENCY、PYTHON_API_MAX_RESPONSE_BYTES、PYTHON_API_BEARER_TOKEN、PYTHON_API_COOKIE 等。它提供 Quote、History、Profile、DataOverview Client 和 Collector；internal/service/wiring.go 使用 Python API Collector，cmd/pythonapi-smoke 可直接探测旧 API。它与 Go Core 的 Python Bridge 是两条不同的迁移链，不能只删除其中一条就宣称 Python 调用清零。

---

## 3. Python Bridge 能力 → Go 映射

| Python 能力 / 函数 | 当前调用方 | Go 已有能力 | 主要缺口 | 目标模块 |
|---|---|---|---|---|
| get_daily_data 日 K | Go Core MarketDataAdapter、旧 Python 主项目 | services/market-data 多 Provider、复权、单位统一、质量校验和存储 | 旧 API Adapter 和所有调用方尚未清零；字段级差分未完成 | Market Data / Provider |
| get_realtime_quote | Go Core MarketDataAdapter、旧 Python 主项目 | Go 多 Provider Quote | Go Core 主链已切 Go；旧 pythonapi Adapter 仍存在 | Market Data |
| get_stock_name | Python 行情 Bridge | Go Security Master、证券搜索和基础资料 | 别名、改名、退市、历史身份需对账 | Security / Market |
| get_chip_distribution | Go Core stock-auxiliary | Go 东方财富筹码、妙想可选能力 | 仍由 Bridge 提供；计算口径、源时间、估算标识、PIT 未对齐 | Market Data / Quality |
| get_fundamental_context | Go Core stock-auxiliary | Go BaoStock/Tushare/同花顺等 Provider 能力 | 仍由 Bridge 提供辅助上下文；报告期、公告时间、修订和 PIT 未证明等价 | Fundamentals / Valuation |
| get_belong_boards | Go Core stock-auxiliary | Go 个股板块和行业能力 | 分类体系、历史成分和更新时间需对账 | Security / Industry |
| get_market_overview | Go Core MarketReviewFlow | Go 指数、市场宽度、行业/概念排行 | CN 原生数据不可用时回退 Python；非 CN 区域仍使用旧 Bridge | Market / Research |
| 新闻搜索 | Go Core SearchAdapter、旧 Python 分析/Agent | Go newssearch 远程搜索、可信域、时间窗口和缓存 | Python fallback 未清零；搜索不等于新闻采集/事件生命周期 | Research Ingestion / News / Event |
| GeminiAnalyzer.analyze | Go Core AnalyzerAdapter | Go Core 工作流、领域模型、ContextPack、阶段 Trace | 模型路由、提示词、调用、结构化结果解析及旧后处理仍在 Python | LLM Gateway / Research Orchestrator |
| 市场复盘生成 | Go Core AnalyzerAdapter | Go MarketReviewFlow | 报告生成仍在 Python，报告契约需冻结 | LLM Gateway / Research Orchestrator |
| 旧结果后处理 | Python _apply_legacy_stock_result_postprocessing | Go 有行情/趋势/ContextPack 基础模型 | 仍调用筹码可用性、价格位置修正、决策稳定和行动字段填充函数 | Research Orchestrator / Domain |
| 日 K 持久化 | Go Core PersistenceAdapter | Go 行情服务 MariaDB Repository | Bridge 仍可写旧 stock_daily；需确认生产调用和旧表数据迁移 | Persistence / Market Data |
| 分析历史持久化 | Go Core PersistenceAdapter | Go 有分析运行、阶段事件、快照、任务表 | Go Core 仍调用 Python AnalysisRepository；旧历史读端兼容未完成 | Persistence |
| 市场复盘持久化 | Go Core PersistenceAdapter | Go 有分析任务/快照基础设施 | Bridge 仍调用 Python 市场复盘存储函数 | Persistence |
| 通知发送 | Go Core NotificationAdapter | 当前 Go Core 未见等价多渠道实现 | 仍调用 Python src.notification | Notification |
| Bridge ReadyCheck | Go Core 启动装配 | Go 原生依赖健康检查 | Python Bridge 仍是启动硬依赖 | Bootstrap / Observability |

迁移完成不能按“Go 目录里存在对应代码”判定。必须同时证明调用方切换、字段/行为契约、数据所有权、真实运行验收和生产无 Python fallback。

---

## 4. 旧 Python 业务能力全景

以下是固定上游源码中可见的能力分组，目标模块仅表示归属，不表示 Go 已实现。

| 能力域 | 代表源码 | 业务对象 | Go 目标模块 | 当前判断 |
|---|---|---|---|---|
| 数据源与证券基础 | data_provider/**、src/data/** | Provider、证券映射、指数、交易日历、复权 | Provider / Market Data / Security | Go 覆盖较多，差分和调用方清零未完成 |
| 财务/估值/筹码 | data_provider/fundamental_adapter.py、src/services/stock_profile_service.py | 报表、估值、筹码、板块、PIT | Fundamentals / Valuation / Quality | 部分覆盖，PIT 和字段验收未完成 |
| 个股/市场分析 | src/analyzer.py、src/stock_analyzer.py、src/core/pipeline.py | 分析上下文、报告、结果修正 | Research Orchestrator / Domain / LLM | Go Core 部分编排，生成和后处理仍在 Python |
| Agent / Skills / Tools | src/agent/** | Agent、技能、工具调用、对话上下文、风险处理、流式事件 | Research Orchestrator / LLM / Quant | Go 只有基础工作流，行为等价未证明 |
| 模型接入 | src/llm/**、src/agent/llm_adapter.py | 多模型后端、路由、协议、参数、用量 | LLM Gateway / Model Adapter | Python 主导，Go 原生适配需建设 |
| 新闻与情报 | src/search_service.py、src/services/intelligence_service.py | 搜索、资讯源、原文、来源可信度 | Research Ingestion / News | Go 搜索有基础，来源管理和证据持久化需补齐 |
| 事件研究 | src/agent/events.py、src/services/market_hotspot_service.py | 事件聚合、热点、影响路径、证券/行业关联 | Event / Research | DATA-016、RESEARCH-004 尚未完成 |
| 筛选与策略 | src/core/market_strategy.py、src/services/screening/** | 筛选、因子、趋势、策略配置、风险和排序 | Quant / Domain | Go 有基础指标/策略代码，逐策略映射未完成 |
| 回测与评估 | src/core/backtest_engine.py、src/services/backtest_service.py | 回测、费用、滑点、策略评估 | Quant / Backtest | 交易制度和样本外验收需继续核对 |
| 组合与持仓 | src/services/portfolio_*.py、src/repositories/portfolio_repo.py | 组合、交易、现金、仓位、批次、汇率、风险 | Quant / Persistence / API | Go 完整等价能力未证明，范围需确认 |
| 告警与通知 | src/services/alert_*.py、src/notification_sender/** | 规则、触发、静默、去重、多渠道投递 | Notification / Application | Go Core 仍调用 Python 通知 |
| 队列与调度 | src/services/task_queue.py、src/services/runtime_scheduler.py、src/scheduler.py、src/services/alert_worker.py | 异步任务、SSE、去重、定时分析、后台告警 | Research Orchestrator / Bootstrap | Go Core 当前工作流以同步 HTTP 为主，后台任务需逐项映射 |
| 业务 API | api/v1/endpoints/** | Agent、分析、历史、股票、回测、系统、组合、告警、信号、筛选、数据、情报、用量 | API / Transport | Go API 不可按少量接口存在就视为全集覆盖 |
| Bot 入口 | bot/** | 命令、消息路由、钉钉、飞书、Discord | API / Transport / Notification | Go 对应入口未见完整实现，需确认保留平台 |
| 持久化 | src/storage.py、src/repositories/** | SQLite ORM、历史、资讯、会话、Agent、回测、组合、告警、信号 | Persistence | Go 有 MariaDB/Redis 基础设施，业务表迁移未完成 |
| 配置与诊断 | src/config.py、src/services/data_capability_service.py、src/services/run_diagnostics.py | 运行配置、模型/Provider 状态、诊断、日志 | Bootstrap / Diagnostics / Security | Go Provider 诊断存在，模型/通知/Agent 配置诊断仍需迁移 |
| 报告与归档 | src/services/report_renderer.py、src/services/research_artifact_service.py | 模板、报告、研究产物、历史版本 | Research / Persistence | Go 结果契约需覆盖版本、来源快照和可复查性 |
| 桌面/构建 | scripts/pyinstaller_runtime_compat.py、apps/**、webui.py | 桌面打包、静态资源、旧 UI 启动 | Bootstrap / WebUI / Build | 不直接视为服务端业务，需确认旧发行形态是否保留 |

---

## 5. Python 数据持久化

### 5.1 固定上游 SQLite ORM 表

固定上游 src/storage.py 中可检索到 31 个 __tablename__：

| 分组 | 表名 |
|---|---|
| Schema 与行情 | schema_migrations、stock_daily |
| 新闻与情报 | news_intel、intelligence_sources、intelligence_items |
| 财务与筛选 | fundamental_snapshot、screening_runs |
| 分析与回测 | analysis_history、backtest_results、backtest_summaries |
| 组合 | portfolio_accounts、portfolio_trades、portfolio_cash_ledger、portfolio_corporate_actions、portfolio_positions、portfolio_position_lots、portfolio_daily_snapshots、portfolio_fx_rates |
| Agent / 对话 | conversation_messages、conversation_session_states、conversation_summaries、agent_provider_turns、llm_usage |
| 告警 | alert_rules、alert_triggers、alert_notifications、alert_cooldowns |
| 决策与技能评估 | decision_signals、decision_signal_outcomes、decision_signal_feedback、skill_opinion_samples、skill_opinion_outcomes |

这 31 张表是固定上游 ORM 源码定义，不代表目标生产 SQLite 实例一定拥有全部表或全部仍在使用。MIG-008 前必须在目标环境只读核对真实 schema、行数、最近写入时间、索引、外键和数据库文件位置。

### 5.2 Go 当前可见持久化基础

- 行情服务已有 MariaDB Repository 和迁移文件，覆盖行情、复权、PIT、Provider 诊断等数据域。
- internal/adapters/storage/mysqlredis 提供 MariaDB + Redis Store；其 schema 检查要求 analysis_runs、analysis_stage_events、analysis_snapshots、analysis_tasks 四张表。
- Go Store 的 Redis 自检使用统一 Key Prefix，临时 Key 为 self-test:<runID>，TTL 30 秒并在自检后删除。
- 但 Go Core 的 PersistenceAdapter 当前仍调用 Python Bridge 写旧 stock_daily、分析历史和市场复盘。

因此，Go 有数据库连接和部分 Repository，不等于旧 Python 业务表已经迁移。MIG-008 需要旧表 → Go Repository/新表映射、转换、幂等、读写切换、回滚和保留策略。

### 5.3 Redis Key 证据边界

- 固定上游 src/storage.py 未发现 Redis 访问或 Key 定义；该文件实现 SQLite ORM。
- 当前 Go Store 可见统一 Key Prefix 和自检临时 Key。
- 本轮没有对生产 Redis 执行 SCAN、INFO keyspace 或读取实例配置，不能断言旧系统完全没有 Redis Key，也不能给出生产 Key 清单。
- MIG-001 仍需目标环境只读核对 Key 前缀、类型、TTL、用途、写入方、读取方和容量；禁止在盘点时清理或改写 Key。

---

## 6. 配置、依赖、部署和 CI

### 6.1 固定旧 Python 上游依赖分类

固定上游 requirements.txt 可见依赖：

| 类别 | 代表依赖 | Go 迁移关注点 |
|---|---|---|
| 配置/重试/调度 | python-dotenv、tenacity、schedule、exchange-calendars | Go 配置、重试、调度和交易日历 |
| 行情数据源 | efinance、akshare、tushare、pytdx、baostock、yfinance、tickflow、longbridge、futu-api | Go Provider 能力、权限、单位和质量验收 |
| 数据处理 | pandas、numpy、pypinyin、openpyxl、orjson、json-repair | Go 数值/表格/导入解析 |
| LLM | litellm、openai、tiktoken、PyYAML | Go Model Adapter、Token/成本记录、配置解析 |
| 搜索/网页 | tavily-python、google-search-results、newspaper3k、requests、httpx | Go 搜索、内容抽取、超时和错误分类 |
| Web/API | fastapi、uvicorn、python-multipart | Go HTTP API / Transport |
| 通知与 Bot | lark-oapi、dingtalk-stream、discord.py、PyNaCl | Go 渠道 Adapter、签名校验、回调和重试 |
| 报告 | markdown2、imgkit、jinja2、lxml_html_clean | Go 报告渲染或独立前端资源；外部 wkhtmltoimage 等依赖需核验 |
| 其他外部能力 | fxmacrodata-public-client | 按项目范围决定，不自动迁移或保留 |

这是固定上游依赖，不是当前 Go 仓库依赖。不能因包名出现在旧清单就直接给 Go 引入替代包；必须先确认生产调用点和仍需保留的业务。

### 6.2 当前 Go 仓库部署与 CI

| 文件 | 当前事实 | 迁移影响 |
|---|---|---|
| 根 docker-compose.yml | 启动 Go 行情服务、Go Analysis API、WebUI；没有 Python Bridge 或 Go Core | 当前统一部署入口与迁移 profile 是两套拓扑 |
| docker/docker-compose.go-migration.yml | go-migration profile 启动 Python Bridge、Go Core、Go 行情服务；Go Core depends_on Python Bridge healthy | 迁移拓扑显式依赖 Python 容器 |
| python_bridge/Dockerfile | Python 3.12，执行 COPY requirements.txt 并启动 Uvicorn 8090 | 依赖旧 requirements 和源码上下文 |
| go-core/.env.example | 含 PYTHON_BRIDGE_BASE_URL、PYTHON_BRIDGE_TOKEN、PYTHON_BRIDGE_TIMEOUT | 退役时删除并做全仓配置扫描 |
| 根 .env.example | Go 行情、MariaDB、Redis、Go 原生新闻搜索等配置 | 需核对实际生产 .env，不能把模板当生产状态 |
| scripts/verify-go-migration.sh | Go test/vet/build 外仍运行 Python compileall 和 pytest | 最终退役后替换 Python 验证步骤 |
| .github/workflows/data-quality-parity.yml | CI 安装 Python 3.12 并运行 Python 对比脚本/测试 | 固定样例应转为脱离 Python 运行时的 JSON/Go 测试 |
| .github/workflows/go-test.yml | Go 测试覆盖根模块、go-core、market-data；构建根 Go API 和行情镜像 | 当前 workflow 未构建迁移 profile 的 Python Bridge 镜像 |

**配置不一致风险：** 当前数据侧分支 Git Tree 没有根目录 requirements.txt，但 python_bridge/Dockerfile 执行 COPY requirements.txt。按当前仓库快照直接构建 Python Bridge 镜像存在文件缺失风险。此结论来自静态路径核对，未执行 Docker build；本轮不修复该配置。

---

## 7. Go 当前替代能力矩阵

| 能力 | Go 状态 | 关键缺口 |
|---|---|---|
| A 股 Quote / DailyBar | 已有主实现 | 旧 Python API Adapter 和全部调用方尚未清零 |
| 证券/行业/市场宽度 | 部分实现 | 历史成分、身份生命周期、覆盖率和旧调用方需对账 |
| 财务/估值 | 部分实现 | PIT、修订、报告期和字段级验收未完成 |
| 筹码/板块辅助 | 部分实现 | Go Core 仍请求 Python stock-auxiliary |
| 新闻搜索 | 部分实现 | Go 原生搜索仍保留 Python fallback |
| 新闻采集/事件 | 未见完整等价实现 | DATA-016、RESEARCH-004 尚未完成 |
| LLM/报告生成 | 未见等价 Go 实现 | AnalyzerAdapter 仍调用 Python Bridge |
| Agent/Skills/工具编排 | 部分实现 | Go Core 基础工作流不能证明覆盖旧上游全部工具/技能 |
| 持久化 | 部分实现 | Go 有 MariaDB/Redis 基础设施，Go Core 业务持久化仍调用 Python |
| 通知 | 未见等价 Go 实现 | NotificationAdapter 仍调用 Python Bridge |
| 任务队列/调度/告警 Worker | 部分实现或未确认 | Go 有分析任务/阶段表；旧任务队列、SSE、调度和 Worker 尚未逐项映射 |
| 旧 API / Bot / 桌面入口 | 未见完整等价实现 | 需要确认仍需保留的入口和平台 |

---

## 8. MIG-001 阶段性结论

### 8.1 已完成的静态工作

- [x] 核对总规划分支、数据侧分支最新 SHA。
- [x] 获取数据侧完整 Git Tree，定位 Python/Go Bridge、旧 API Adapter、部署和 CI 入口。
- [x] 读取 Bridge 路由、能力适配器、Go Client 和相关测试。
- [x] 读取 Go Core 启动装配、Ports、工作流和各 Python Adapter。
- [x] 核对 Go 原生新闻搜索、行情服务、MariaDB/Redis Store。
- [x] 读取固定旧 Python 上游目录、requirements、SQLite ORM 表、FastAPI 路由和调度/任务入口。
- [x] 建立 Python 能力与 Go 目标模块的初步映射。
- [x] 发现 Bridge Dockerfile 对根 requirements.txt 的静态路径依赖风险。

### 8.2 尚未完成的内容

| 待补证项 | 所需证据 | 状态 |
|---|---|---|
| 目标服务器真实进程/容器 | 只读容器、镜像、启动命令、挂载、网络、健康状态 | 待运行时补证 |
| 实际 Python 入口和调用量 | 网关/反向代理配置、访问日志、调用方清单 | 待运行时补证 |
| 生产环境变量 | 只读变量名及脱敏配置状态，禁止输出 Secret | 待运行时补证 |
| Python 任务与调度 | 运行配置、进程日志、任务执行记录 | 待运行时补证 |
| SQLite 实际文件 | 文件位置、只读 schema、表行数和最近写入时间 | 待运行时补证 |
| Redis 实际 Key | Key 前缀、类型、TTL、用途和读写方 | 待运行时补证 |
| Python 上游部署版本 | 镜像标签、构建 SHA、运行目录或版本端点 | 待运行时补证 |
| API/Bot/桌面调用方 | 生产路由、机器人配置、客户端和实际使用范围 | 待运行时补证 / 待用户确认 |
| 配置项使用状态 | 配置字段 → 读取代码 → 部署值 → 实际启用状态 | 待补充代码扫描 |

**MIG-001 完成：否。** 静态源码盘点已经形成，但缺少目标运行环境证据、实际调用量、数据库实例、Redis Key 和旧入口使用状态。不得把阶段性静态盘点写成全量完成。

---

## 9. MIG-002 契约冻结建议

| 顺序 | 模块组 | 必须冻结的契约 | 主要依赖 |
|---|---|---|---|
| 1 | 行情/证券/财务/估值/筹码/行业 | 证券身份、单位、复权、时间/PIT、质量状态、来源和分页 | DATA-001~012 |
| 2 | 新闻/情报/事件 | 原始来源、发布时间/采集时间、去重键、可信度、事件生命周期和证券/行业关联 | DATA-016、RESEARCH-004 |
| 3 | LLM/Agent/研究编排 | 模型协议、模型/提示词版本、工具权限、上下文快照、结构化输出、失败恢复、风险门禁 | 统一分析结果契约 |
| 4 | 持久化/历史/任务 | 旧 SQLite 表 → Go Repository/新表映射、主键、幂等、版本、事务、迁移和回滚 | DATA-008 |
| 5 | 通知/告警 | 渠道配置、订阅、规则、去重、重试、投递状态和审计 | CROSS-005 |
| 6 | 调度/队列/API/Bot | 任务状态机、并发、取消、SSE/事件、重试、鉴权、兼容路径和调用方 | Go API / Research Orchestrator |
| 7 | 旧 UI/桌面/构建入口 | 是否保留、WebUI 接管范围、静态资源和打包策略 | WebUI 规划及用户确认 |

每项映射必须记录旧模块/函数/API、真实调用方、Go 当前代码路径、输入输出字段、单位/时间/空值/错误语义、数据所有权、旧表/Key、fallback 条件、固定样本、切流回滚和删除条件。

---

## 10. 后续迁移顺序

1. MIG-001 补齐运行时只读清点、外部调用方、SQLite 实例、Redis Key、生产配置和容器 SHA。
2. MIG-002 冻结逐函数/路由/表级映射，区分复用已有 Go 实现和必须新增的能力。
3. MIG-003 记录现存 Python fallback 的调用点、触发条件、日志和开关；禁止新增 Python 生产依赖，但不在替代验收前强制关闭必要链路。
4. MIG-004 收口行情/财务/估值/市场数据；Go shares/CNY、null/zero、复权和质量诊断改进必须保留。
5. MIG-005/006 迁移新闻采集、原始证据、去重、事件生命周期和历史关联。
6. MIG-007 迁移模型适配、提示词、工具调用、结构化输出、旧结果后处理、任务状态机和失败恢复。
7. MIG-008 迁移持久化；先做旧表到 Go Repository/新表映射和数据迁移演练，再读写切换。
8. MIG-009 迁移通知渠道、订阅、模板、幂等、重试和投递记录。
9. 依据 MIG-002 冻结结果迁移仍需保留的 API、Bot、Scheduler、Worker 等入口。
10. MIG-010 固定样本差分；Python 的补零、未知单位等缺陷不得成为 Go 回归标准。
11. MIG-011 逐模块确认生产调用量为零、调用方已切 Go、回滚窗口关闭后，拆除 Bridge Adapter。
12. MIG-012 删除 Python 生产容器、requirements/锁文件、部署配置、Python CI 依赖和不再需要的脚本；保留脱离 Python 运行时的黄金 JSON 样例。

---

## 11. 必须保留的 Go 数据质量改进

- 成交量统一为标准 shares，成交额统一为标准 CNY；无供应商证据时保持 unknown/degraded。
- null、真实 0、未知值必须区分；不得把缺失成交量/成交额补成 0。
- 复权基准区分 none、forward_adjusted、backward_adjusted、unknown；不得混拼不同算法。
- 保留 source、source_version、provider_time、fetched_at、normalized_at、quality_status、quality_reasons 和 fallback 证据。
- 非有限数值、非法 OHLC、重复/越界日期、异常数量级、分页/覆盖不完整等校验不得绕过。
- 旧 Python 字段结构兼容只允许发生在 API/Adapter 边界，不得污染 Go 内部标准模型。
- 旧 Python 缺陷应记录为“Go 更严格”或“可解释差异”，不能作为 Go 目标行为。

---

## 12. 参考文件

### 当前 Go 仓库（数据侧基线 d91b2abf1fbc2a55e5db07ac06097a42e3436342）

- [Python Bridge 应用](https://github.com/Kasign/stock_data_provider/blob/feat/full-go-data-providers-api-switch/python_bridge/capability_app.py)
- [Python Bridge 行情适配](https://github.com/Kasign/stock_data_provider/blob/feat/full-go-data-providers-api-switch/python_bridge/capabilities/market_data.py)
- [Python Bridge LLM 适配](https://github.com/Kasign/stock_data_provider/blob/feat/full-go-data-providers-api-switch/python_bridge/capabilities/llm.py)
- [Python Bridge 持久化适配](https://github.com/Kasign/stock_data_provider/blob/feat/full-go-data-providers-api-switch/python_bridge/capabilities/persistence.py)
- [Python Bridge 通知适配](https://github.com/Kasign/stock_data_provider/blob/feat/full-go-data-providers-api-switch/python_bridge/capabilities/notification.py)
- [Go Core 启动装配](https://github.com/Kasign/stock_data_provider/blob/feat/full-go-data-providers-api-switch/go-core/cmd/analysisd/main.go)
- [Go Core Python Bridge Adapter](https://github.com/Kasign/stock_data_provider/tree/feat/full-go-data-providers-api-switch/go-core/internal/adapters/pythonbridge)
- [旧 Python API Adapter](https://github.com/Kasign/stock_data_provider/tree/feat/full-go-data-providers-api-switch/internal/adapters/pythonapi)
- [迁移 Compose Profile](https://github.com/Kasign/stock_data_provider/blob/feat/full-go-data-providers-api-switch/docker/docker-compose.go-migration.yml)
- [Go/Python 迁移矩阵](https://github.com/Kasign/stock_data_provider/blob/feat/full-go-data-providers-api-switch/doc/api/接口规范/PYTHON_PROVIDER_GO_MIGRATION_MATRIX.md)
- [Go/Python 数据质量对账报告](https://github.com/Kasign/stock_data_provider/blob/feat/full-go-data-providers-api-switch/doc/api/接口规范/GO_PYTHON_DATA_QUALITY_PARITY_REPORT.md)

### 固定旧 Python 上游（迁移期对照基准）

- [固定上游提交](https://github.com/Kasign/daily_stock_analysis/commit/7a21cd76952c7fe44a7fc5a6113a5b2b8a16bb32)
- [旧 Python 存储](https://github.com/Kasign/daily_stock_analysis/blob/7a21cd76952c7fe44a7fc5a6113a5b2b8a16bb32/src/storage.py)
- [旧 Python 配置](https://github.com/Kasign/daily_stock_analysis/blob/7a21cd76952c7fe44a7fc5a6113a5b2b8a16bb32/src/config.py)
- [旧 Python API 路由](https://github.com/Kasign/daily_stock_analysis/blob/7a21cd76952c7fe44a7fc5a6113a5b2b8a16bb32/api/v1/router.py)
- [旧 Python 依赖](https://github.com/Kasign/daily_stock_analysis/blob/7a21cd76952c7fe44a7fc5a6113a5b2b8a16bb32/requirements.txt)

---

## 13. 更新记录

- 2026-11-04：首次静态盘点；核对当前数据侧 HEAD、Git Tree、Go/Python Bridge 调用拓扑、固定旧 Python 上游、业务模块、SQLite ORM 表、依赖、部署和 CI；MIG-001 保持未完成，等待目标运行环境补证。
