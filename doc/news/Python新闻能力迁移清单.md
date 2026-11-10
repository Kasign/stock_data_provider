# 旧 Python 新闻能力盘点与历史数据评估

> 状态：历史盘点资料；不构成兼容开发要求
>
> 阅读规则：本文件前半部分的接口、开关、部署和迁移描述是旧系统事实快照，不是新系统待办。第 11 节为当前最终决策；与其冲突的旧灰度、兼容、双写和调用方切换步骤全部废弃。  
> 原则：先建立调用链证据，再确定迁移范围；不得根据模块名称推断调用关系。

## 1. 迁移原则

本专项不实现旧 Python 服务兼容层，不删除或修改独立旧 Python 服务；旧实现仅作为历史行为和数据语义参考。

Python 旧实现不是新模块的验收基准；新模块按已冻结的业务契约和质量规则验收。

## 2. 历史调用信息记录字段（仅用于理解旧系统，不用于新模块兼容实现）

| 项目 | 记录要求 |
|---|---|
| Python 文件/函数 | 精确路径和入口 |
| 调用方 | 服务、任务、API 或定时任务 |
| 触发条件 | 请求、任务类型、开关或调度条件 |
| 当前能力 | 搜索、过滤、解析、去重、关联等 |
| 输入/输出契约 | 字段、错误、时间和状态 |
| Go 替代位置 | 对应 Adapter/Application/API |
| 兼容开关 | 配置项及默认行为 |
| 业务结果 | 结构化错误、部分成功、质量原因；通用日志/指标/诊断由公共服务提供 |
| 迁移状态 | 未盘点/待实现/验证中/灰度/完成 |
| 退役条件 | 可量化验收条件和责任范围 |

## 3. 迁移步骤

1. 完整搜索 Python 代码和所有调用方。
2. 为每条链路建立输入输出契约和固定样本。
3. 若未来决定迁移历史数据，单独评估授权、隐私、完整性和公共存储成本。
4. 不做运行时双写、灰度切换或兼容 fallback。
5. 新模块只按自身契约执行单元、契约和集成测试。

## 4. 当前盘点表

N1 核查后逐项填写，未核查前不填推测结果。

| 链路/文件 | 调用方 | 触发条件 | Go 替代 | 状态 | 证据 |
|---|---|---|---|---|---|
| 待 N1 盘点 | 待核实 | 待核实 | 待核实 | 未盘点 | - |

## 5. 退役门槛

- 所有调用方已登记。
- Go 契约和固定样本测试通过。
- 数据质量和时间语义通过验收。
- 真实环境验证通过。
- 灰度期无关键回归。
- 监控证明旧链路无实际调用。
- 回滚路径明确。
- 用户明确批准退役。

---

## 6. N1 已核实调用链（2026-11-05）

| 链路/文件 | 调用方 | 触发条件 | 当前能力 | Go 替代/边界 | 状态 |
|---|---|---|---|---|---|
| python_bridge/capabilities/search.py::search_stock_news | capability_app.py 内部路由 | POST /internal/v1/search/stock-news | 调用 SearchService 多维检索并扁平化结果 | Go Core SearchAdapter 调用该路由；原生 Go Searcher 可用时优先 | 已核实 |
| python_bridge/capabilities/search.py::search_market_news | Bridge 内部市场新闻路由 | POST /internal/v1/search/market-news | 调用 MarketAnalyzer.search_market_news() | Go Core SearchAdapter 调用该路由；原生 Go Searcher fallback | 已核实 |
| go-core/internal/adapters/newssearch/search.go | go-core/cmd/analysisd | 可配置原生 Provider | Bocha/Tavily/Brave/SearXNG、公告源、缓存、过滤排序、Python fallback | 搜索适配器保留；不等于新闻采集持久化 | 已核实 |
| src/agent/tools/search_tools.py::_handle_search_stock_news | 旧 Python Agent Tool Registry | Agent 调用 search_stock_news | 单维新闻搜索、记录证据数量、保存结果 | Go Core 基础搜索接口不代表 Agent tool 行为完全迁移 | 已发现，调用方范围待追踪 |
| src/agent/tools/search_tools.py::_handle_search_comprehensive_intel | 旧 Python Agent Tool Registry | Agent 调用 comprehensive intel | 多维检索、报告格式化、按维度持久化 | Go Core NewsItem 列表不等价于多维 Agent 工具输出 | 已发现，调用方范围待追踪 |
| src/agent/tools/search_tools.py::_persist_news_response | 上述 Agent 工具 | Provider 成功且有结果 | 调用 DB save_news_intel 持久化 | 新新闻库需明确替代旧情报存储的范围 | 已核实方法，数据库实现与读取调用仍待追踪 |
| src/market_analyzer.py 的本地情报合并 | MarketAnalyzer | 市场复盘生成时 | 将已持久化市场情报与搜索结果合并 | 需判断是迁移新闻查询还是保留研究素材库语义 | 已发现，具体调用链待追踪 |

### 6.1 旧系统部署与兼容开关（历史事实，不迁移）

- 根目录统一 Compose 只运行 stock-analysis-server、stock-analysis-data、stock-analysis-web，不包含 Python Bridge 和 go-core。
- 可选 docker/docker-compose.go-migration.yml 才运行 Python Bridge + go-core + market-data；其注释明确与根统一栈分开。
- go-core/cmd/analysisd 需要 PYTHON_BRIDGE_BASE_URL、PYTHON_BRIDGE_TOKEN；Python Bridge 使用 X-Python-Bridge-Token。
- python_bridge/go_core_client.py 的 GO_CORE_ENABLED 默认 false，且仅封装分析工作流，不提供新闻持久化迁移开关。
- 因此，不能仅根据 Go Core 已有 SearchAdapter 就判断当前统一部署已切换到 Go 新闻链路。需要确认线上实际采用哪套 Compose/服务。

### 6.2 迁移缺口

1. Go Core 目前只把搜索结果传给分析流程，不提供新闻实体入库、原始正文、版本历史、来源目录和新闻查询 API。
2. Python Agent 工具具有额外的多维格式化、证据计数和 save_news_intel 持久化行为；不能用 Go 的简单搜索结果列表宣称完全等价。
3. 市场复盘会读取已持久化的市场情报，需继续追踪其 repository/schema 和刷新来源。
4. 当前 Bridge 两个搜索路由的成功状态不代表来源采集、内容解析或持久化成功。
5. 旧 Python API/Bot/Scheduler 的直接调用方尚未穷尽；N1 需要完成全仓调用引用与触发入口盘点后才能制定退役条件。

---

## 7. 旧 Python 上游的持久化与入口补充（历史事实）

固定提交 daily_stock_analysis@7a21cd76952c7fe44a7fc5a6113a5b2b8a16bb32 中发现三类新闻/情报持久化对象：
- news_intel：Agent 搜索结果，唯一键为 URL，包含 query/provider/dimension、证券代码、查询来源与请求上下文。
- intelligence_sources：RSS/Atom/NewsNow 来源配置、作用范围、启停和最近采集状态。
- intelligence_items：已采集的来源条目，唯一键按来源、URL、scope、market 组合，保存 raw_payload；Repository 有分页查询、upsert 和 fetched_at 保留期清理。

旧实现已经有来源管理、dry-run、单来源/批量采集、自动刷新、分页读取、URL/DNS 校验和重定向限制；但 IntelligenceRepository.upsert_items 对同一条目是覆盖更新，不保留修订历史。Go 迁移需要保留这些有效业务能力，同时补足内容版本、可追溯去重证据和更清晰的采集阶段状态。

已核实旧 API 路由位于 api/v1/endpoints/intelligence.py，覆盖 sources CRUD/list/templates/defaults/test/fetch、fetch-enabled 和 items list；旧 Bot /research 入口会构造 ResearchAgent 并调用 Tool Registry。MarketAnalyzer 会自动刷新并查询 market scope 的 intelligence_items；Agent search tools 则写入 news_intel。两套旧持久化语义不能未经映射直接合并。

---

## 8. 旧 Python 调用链与存储迁移补充（历史事实）

固定旧上游提交 `Kasign/daily_stock_analysis@7a21cd76952c7fe44a7fc5a6113a5b2b8a16bb32` 的调用链：
- API：`/api/v1/intelligence/**` 来源管理、dry-run、采集、条目查询；`POST /api/v1/agent/research` 间接调用 Agent 搜索工具。
- Bot：`/research` 调用 ResearchAgent；Agent ToolRegistry 注册搜索工具。
- 个股分析：`src/core/pipeline.py` 直接调用多维 SearchService，写入 `news_intel`；分析前可刷新并读取 `intelligence_items`。
- 大盘复盘：`src/market_analyzer.py` 搜索市场新闻，刷新并读取 market scope 资讯。
- 定时任务并非单独的新闻 Provider；当其触发上述分析流程时，可能间接触发自动刷新。该能力由 `NEWS_INTEL_AUTO_FETCH_ENABLED` 控制，默认关闭。

旧存储默认配置为 SQLite 文件 `./data/stock_analysis.db`，使用 SQLAlchemy ORM；`src/storage.py` 定义 `NewsIntel`、`IntelligenceSource`、`IntelligenceItem`。旧表含用户/会话请求上下文和本地作用域，不能只迁移标题/URL 而忽略其查询证据关系。Go 新库为 MariaDB，迁移应单独设计 SQLite 导出、幂等导入、冲突报告、计数校验和回滚，不在运行时双写两个数据库。

旧模型：
- `news_intel`：搜索证据及请求上下文，URL 全局唯一。
- `intelligence_sources`：可配置 RSS/Atom/NewsNow 来源。
- `intelligence_items`：来源采集条目，按 source/url/scope/market 组合唯一，保留 raw_payload。


## 9. 历史迁移分析边界（不构成当前实施计划）

- 该段描述的 DTO 转换、兼容适配和调用方切换方案已废弃；当前只保留调用链盘点、旧字段语义分析和历史数据价值评估。
- 不实现 Python 旧 SQLite 到 MariaDB 的正式导入，不做双写，不创建临时数据库连接或 Redis 缓存。
- 旧数据迁移预览、幂等批次、冲突报告、匿名化、计数校验和回滚方案可继续设计；实际执行等待公共 DB/迁移服务契约就绪。
- 通用诊断与历史观测由公共服务统一承接；新闻迁移阶段只保留业务错误、质量原因和切换结果，不自行建设诊断平台。
- 本专项不负责维持旧 Python 调用链可用，也不以 Go 替代能力验收作为旧服务切换条件；旧服务运行与退役由其所属项目独立决定。


## 10. 旧 Python 三类数据的潜在一次性导入价值评估（未批准执行）

三类旧数据语义不同，不合并为一个无差别 news_item：

### 10.1 news_intel：历史搜索发现证据

- 定位为旧 Agent/SearchService 搜索发现记录和研究请求上下文，不直接认定为已核实新闻主体。
- 旧表 URL 全局唯一，命中后会覆盖部分 query/provider/dimension/snippet/source/published_date 等字段；因此现存行是最后一次覆盖后的快照，不是完整搜索历史，也不能恢复所有曾经的候选版本。
- 若未来导入，优先映射为来源为 legacy_news_intel 的历史 observation；`news_id` 默认为空。只有在 publisher source、身份凭据和保存权利均核验通过后，才允许关联到正式 news_item。
- provider 字段必须保留为历史 provider_code；如果为空或无法识别，标记 legacy_provider_unknown，不自动归入某个 Provider。
- query_id、query、dimension、query_source 等可作为最小研究上下文保留；不得把 requester_user_id、requester_user_name、requester_chat_id、requester_message_id 等个人/会话标识迁入公共新闻事实。确需追溯时另行设计匿名化的审计映射，默认不迁移。
- 旧记录的来源/Provider 条款版本未必可追溯；无法证明历史输出可留存时，先保留在旧库或隔离清单，不直接写入新库。

### 10.2 intelligence_sources：来源目录与旧配置

- 映射目标是新来源身份和 endpoint 配置，而不是新闻主体。
- 旧 source_type、URL、名称、scope_type/scope_value、market、description 仅作为未来人工审核时的历史参考，不得直接导入并自动启用。
- 旧 enabled 仅表示旧系统是否启用，不代表新系统授权通过；迁移后新授权状态统一初始化为 pending，需重新核验条款、robots 和内容保存范围。
- 旧 last_status、last_error、last_fetched_at 只能作为 legacy 状态快照，不能冒充当前来源可用性或公共诊断记录。
- source_name 无法可靠解析为真实发布者时进入待人工映射清单，不创建通用 unknown source。
- 旧模板和默认源仅是配置候选，不自动启用。

### 10.3 intelligence_items：来源采集条目

- 定位为旧 RSS/Atom/NewsNow 等来源采集的条目快照，与 news_intel 的搜索发现记录分开处理。
- 可映射为历史 observation；只有来源身份、URL 身份、授权和内容质量通过规则后，才创建正式 news_item/version。
- `source_id` 非空时先映射到已确认的新 source；`source_id` 为空时尝试按 source_name + source_type + URL host 生成待审核映射，不自动合并。
- 旧唯一键包含 source、URL、scope、market。同一 source + URL 因 scope 不同出现多行时，优先形成一个新闻主体和多条适用范围/关联证据，不机械复制新闻实体。
- 旧 raw_payload 常只有 `{source: source_name}`，不能视为完整原始 RSS/XML/HTML 证据；必须按实际字段内容判断，禁止在迁移报告中夸大原始证据完整度。
- 旧 upsert 覆盖 summary/source/published_at/fetched_at/raw_payload，不保留历史修订；新版本只能从迁移时仍存在的最后快照建立，不能伪造旧版本链。
- published_at、fetched_at 保留原始值和时区/解析不确定性；不得根据迁移时间补造历史可用时间。

### 10.4 迁移执行顺序（等待公共 DB/迁移服务）

1. 只读导出旧 SQLite，生成表级计数、字段缺失、URL 冲突、来源映射和隐私字段报告。
2. 先迁移/审核来源目录，所有新来源默认 pending。
3. 分别预览 news_intel 和 intelligence_items，输出不同的映射计划。
4. 通过授权审核和数据质量门槛后，按批次幂等导入 observation 与可确认的 news_item/version。
5. 做导入前后计数、抽样内容、关联和重复冲突核验。
6. 保留批次映射表和可逆的导入清单；出现问题时停止后续批次并按批次回滚。
7. 若未来单独批准历史导入，只执行可审计的一次性批次导入；不做运行时双写、调用方灰度或双运行。旧服务是否停止由其所属项目另行决定。

本节是领域迁移决策建议，不表示已执行导出、导入或任何数据库操作。正式执行等待公共 DB/迁移能力就绪。


## 11. 最终实施边界（2026-11-05）

用户已明确整体服务尚未开始使用，不需要为了旧服务兼容而增加开发工作。本文件从此定位为历史调用链、旧数据语义和潜在历史数据价值的参考盘点，不再是 Go 新闻模块的兼容迁移计划。

- 不实现 Python Bridge fallback、旧 NewsItem DTO 转换、Agent/Bot/API 兼容或调用方灰度切换。
- 不要求旧 Python 调用链在新模块交付期间继续可用，也不以旧 Python 行为作为新模块正确性标准。
- 不实现旧 SQLite 双写、在线同步、兼容 Repository 或旧表结构映射层。
- 旧三类数据的分析仅用于判断未来是否值得一次性导入；导入须等公共 DB/迁移服务就绪，并单独确认数据价值、授权、隐私和迁移成本。
- 不删除旧 Python 仓库或服务；本新闻专项不负责其维护和退役。
- 新新闻 Domain/Application 不为旧数据字段、用户会话字段或旧 API 结构让步。

因此，前文关于“先保留旧链路、灰度切换、无调用后退役”的步骤仅描述历史迁移背景，不再是当前实施门槛。