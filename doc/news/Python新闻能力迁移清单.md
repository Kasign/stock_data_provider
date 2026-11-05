# Python 新闻能力迁移清单

> 状态：待 N1 代码追踪  
> 原则：先建立调用链证据，再确定迁移范围；不得根据模块名称推断调用关系。

## 1. 迁移原则

Go 替代能力验收前，不删除 Python Bridge、旧服务、旧调用方或兼容配置。

Python 旧实现可以作为固定样本对照基准，但不能将旧实现的缺陷视为 Go 的正确行为。

## 2. 必须登记的调用信息

| 项目 | 记录要求 |
|---|---|
| Python 文件/函数 | 精确路径和入口 |
| 调用方 | 服务、任务、API 或定时任务 |
| 触发条件 | 请求、任务类型、开关或调度条件 |
| 当前能力 | 搜索、过滤、解析、去重、关联等 |
| 输入/输出契约 | 字段、错误、时间和状态 |
| Go 替代位置 | 对应 Adapter/Application/API |
| 兼容开关 | 配置项及默认行为 |
| 可观测性 | 日志、指标和诊断 |
| 迁移状态 | 未盘点/待实现/验证中/灰度/完成 |
| 退役条件 | 可量化验收条件和责任范围 |

## 3. 迁移步骤

1. 完整搜索 Python 代码和所有调用方。
2. 为每条链路建立输入输出契约和固定样本。
3. 实现 Go 替代能力，保留旧链路。
4. 对比功能、时间语义、来源证据、质量和异常行为。
5. 运行单元、契约、集成和真实环境验证。
6. 通过受控开关灰度切换。
7. 观察日志、指标和业务结果。
8. 确认无旧调用后，提出退役方案。
9. 经明确批准后再删除旧实现和配置。

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

### 6.1 部署与兼容开关

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
