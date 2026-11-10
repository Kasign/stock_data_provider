# PLAT-002 审计基线

## 1. 固定代码基线

| 项目 | 值 |
|---|---|
| 仓库 | `Kasign/stock_data_provider` |
| 代码分支 | `origin/integration` |
| 代码 HEAD | `b6dfc85ba214fe16268c8cba780641a8bfe15086` |
| 审计文档分支 | `origin/feat/design_doc` |
| 文档分支 HEAD（基线建立时） | `671b76db8879766f3b43990406eb0a1216a53424` |
| 分支比较 | integration...feat/design_doc：ahead 7、behind 0 |
| 比较范围 | 14 个变更文件均在 `doc/`，无业务代码变更 |
| 审计日期 | 2026-11-10 |
| 审计状态 | review_in_progress，未冻结 |

本轮所有代码事实均绑定 integration 的固定 SHA `b6dfc85ba214fe16268c8cba780641a8bfe15086`。审计期间 integration 前进时，不自动改变本轮结论；新增提交进入变更观察清单，并在必要时安排增量复核。

## 2. 正式模块清单

| 模块 | 代码/资源位置 | 核验边界 |
|---|---|---|
| Platform | `internal/platform/**` | 公共技术原语，不承载领域业务语义 |
| Server | `cmd/**`、`internal/server/**` | 唯一 Root Composition Root、HTTP 宿主和生命周期装配 |
| Data | `internal/marketdata/**`、`services/market-data/**` | 市场数据 Application、Provider、Repository、旧独立入口 |
| News | `internal/news/**` | 新闻领域、证据查询与授权 |
| Research | `internal/research/**` | 研究领域、证据与研究资源授权 |
| Researchflow | `internal/researchflow/**` | 研究流程编排、Worker、Scheduler 和跨模块 Port |
| Quant | `internal/quant/**` | 确定性计算、结果与授权复核 |
| LLM | `internal/llm/**`、`internal/server/llmtransport/**` | 模型协议适配与受控传输装配 |
| Notification | `internal/notification/**` | 事件接收、投递状态、Dispatcher/Worker |
| WebUI | `web/**` | 浏览器端 DTO 消费、身份凭据和 HTTP 请求边界 |

十模块清单来自项目既有 PLAT-002 治理上下文；代码路径来自本次 integration tree。目录不存在或代码不在预期位置时，作为核验差异记录，不擅自改写模块清单。

## 3. 基线结构盘点

Git tree：1,281 个文件条目，未截断。

| 模块 | 生产 Go 文件 | Go 测试文件 |
|---|---:|---:|
| Platform | 24 | 32 |
| Server | 16 | 16 |
| Data | 107 | 157 |
| News | 60 | 51 |
| Research | 26 | 19 |
| Researchflow | 47 | 43 |
| Quant | 21 | 29 |
| LLM | 9 | 7 |
| Notification | 48 | 50 |
| WebUI | JavaScript/HTML/CSS，非 Go Module | 另有 web/tests |

其他结构事实：
- 根目录存在 `go.mod` / `go.sum`。
- 同时存在独立 `go-core/go.mod`。
- 存在 `cmd/analysis-api` 与 `cmd/market-data` 两个启动入口。
- 存在 `services/market-data/**` 部署目录。
- WebUI 位于 `web/**`，不在 `internal/**` 下。

文件数量只用于确定审计规模，不代表质量或完成度。

## 4. 并行修改观察

用户说明当前有四个模块正在修改细节，预期不影响契约。本轮先将以下模块列为变更观察对象：

| 模块 | 当前处理 | 纳入本轮基线？ |
|---|---|---|
| Server | 继续审计固定 SHA；未合入改动另行观察 | 否 |
| Data | 继续审计固定 SHA；未合入改动另行观察 | 否 |
| Researchflow | 继续审计固定 SHA；未合入改动另行观察 | 否 |
| WebUI | 继续审计固定 SHA；未合入改动另行观察 | 否 |

此处是审计范围管理，不表示已独立审查四个模块当前工作区。后续只有当改动涉及六维契约边界时才建立增量复核任务。

## 5. 基线限制

- 此基线不是各模块开发分支的最新状态。
- 本轮不把未合入改动推断为无契约影响；只按用户说明先作为细节改动观察。
- 当前文件盘点未完成全仓 import graph、Port 消费者追踪、Schema Validator、Golden 复算或端到端验收。
- 本基线不构成任何契约冻结结论。
