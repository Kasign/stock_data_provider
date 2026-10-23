# WebUI 实施进度

更新时间：2026-10-23  
目标分支：feat/webui-market-data-console  
范围：只修改 web/ 前端、前端测试与相关文档；本轮不改 Go API、数据源、数据库或任务契约。

## 本轮已完成的代码工作

| 项目 | 状态 | 实现说明 |
| --- | --- | --- |
| 分析股票代码兼容 | 已实现，待真实请求验收 | 行情查询仍保留 600519.SH 等交易所后缀；提交 Go 分析任务时将代码规范化为后端当前接受的六位数字，并去重。 |
| 行情页直达单股分析 | 已实现，待真实请求验收 | 增加“分析当前股票”入口，读取当前显示标的，填入单股任务并使用现有 post_market_scan 类型提交。 |
| 结构化分析结果 | 已实现，待真实回包验收 | 按 Go RunResult 模型展示请求概况、市场状态、质量报告/问题、因子、候选信号、可交易性、风险、交易计划、计划拒绝、执行评估、失败与警告。 |
| 避免假字段 | 已实现 | 删除基于 score、confidence、reason 等模型未声明字段的猜测性展示；缺失值显示为未报告，不补 0。 |
| 任务状态与 Run 状态 | 已实现 | 分别解释异步任务状态与 Run 状态，包括 blocked_data_quality、rejected_request 等后端状态。 |
| 刷新后恢复最近任务 | 已实现，待浏览器联调 | 浏览器 localStorage 仅保存最近一项 task/run ID、标的、任务类型与状态，重新打开页面后通过现有 task/events/run 接口恢复。 |
| 手动读取和诊断 | 已实现，待浏览器验收 | 提供重新读取 Run、复制完整 JSON、复制任务/质量/失败/事件诊断内容的入口。 |
| 最近分析列表 | 已实现并通过 CI | 当前浏览器内保存最近 10 条 task/run 引用；点击记录会通过现有 task/events/run 接口恢复，不保存完整报告，也不是服务端历史。验证见 [WebUI CI #39895905854](https://github.com/Kasign/stock_data_provider/actions/runs/39895905854)。 |
| 报告下载 | 已实现并通过 CI | 增加结构化 Markdown 与原始 JSON 下载，文件名使用标的和分析时点；不增加后端接口。验证见 [WebUI CI #39895905854](https://github.com/Kasign/stock_data_provider/actions/runs/39895905854)。 |
| 错误与缺失值反馈 | 已实现并通过 CI | 401 错误不再默认指向行情 API Key；反馈包含 HTTP 状态、服务端错误码及 request ID（若有），任务创建失败改为页面内联提示，缺失 completed_stages 显示“未报告”。验证见 [WebUI CI #39896321052](https://github.com/Kasign/stock_data_provider/actions/runs/39896321052)。 |
| 前端单元测试 | 通过（19 项测试） | 覆盖股票代码/状态格式化、DOM ID 引用、Go RunResult 字段映射、任务/Run 状态、任务请求体/幂等键、事件分页参数、密钥隔离、错误响应、最近任务存储和报告导出；最新 [WebUI CI #39896321052](https://github.com/Kasign/stock_data_provider/actions/runs/39896321052) 全部通过。 |

## 第二轮 Review 修正

- 避免任务轮询时重复同步写入 localStorage：只有 task/run ID、标的、类型或状态真的变化时才更新。
- Task ID 复制复用统一剪贴板工具，浏览器不支持或权限不足时可尝试降级复制。
- 新增 UI 契约测试，检查 app.js 引用的静态 DOM ID 是否存在且无重复，并检查结构化结果字段与 Go domain JSON 字段对齐，防止误用不存在的 signal score/confidence 字段。
- 增加 API 客户端自动化测试，验证创建任务的请求体/幂等键、事件增量查询参数、行情 API Key 与分析 API 的隔离，以及服务端错误码/请求 ID 的保留；仅测试现有调用，不改后端 API 契约。

## 本轮 WebUI 扩展

- 增加浏览器本地“最近分析”列表，最多 10 条，可选择记录并恢复服务端 task/run 状态；为旧版本仅保存单条任务的 localStorage 记录增加迁移。
- 增加 Markdown 报告与原始 JSON 下载；Markdown 导出明确标记未报告字段，不生成额外评分、置信度或交易结论。
- 新增最近任务存储单元测试和 Markdown/文件名测试。第四轮 [WebUI CI #39895905854](https://github.com/Kasign/stock_data_provider/actions/runs/39895905854) 已通过：JS 语法检查、18 项 Node 测试和 Docker 镜像构建均成功。
- 新增统一 API 错误描述与任务创建内联错误提示；未知的 completed_stages 不显示为 0。第五轮 [WebUI CI #39896321052](https://github.com/Kasign/stock_data_provider/actions/runs/39896321052) 已通过：JS 语法检查、19 项 Node 测试和 Docker 镜像构建均成功。
- 本地记录可能因浏览器存储被清除而消失；记录引用已过期或服务端不可用时，仍需通过界面错误信息判断，不能将其视为服务端归档。

## 明确待完成

1. **GitHub 验证：已完成**。首轮实现 [#39893969836](https://github.com/Kasign/stock_data_provider/actions/runs/39893969836)、第二轮 Review [#39894703741](https://github.com/Kasign/stock_data_provider/actions/runs/39894703741)、第三轮 API 测试 [#39894969470](https://github.com/Kasign/stock_data_provider/actions/runs/39894969470)、第四轮本地历史/报告导出 [#39895905854](https://github.com/Kasign/stock_data_provider/actions/runs/39895905854) 和第五轮错误/缺失值修正 [#39896321052](https://github.com/Kasign/stock_data_provider/actions/runs/39896321052) 均通过；最新轮包含 JS 语法检查、19 项 Node 测试与 Docker 镜像构建。
2. **NAS / 1Panel 真实端到端联调**：在真实 Docker 网络里验证 stock_analysis_web 的 /analysis-api 代理、任务提交、task/run 关联、阶段事件、Run 读取和 API 错误显示。
3. **真实数据场景验收**：至少检查正常数据、部分可选数据缺失/质量降级、后端暂不可用三类情况；逐项比对页面与原始 JSON。
4. **浏览器刷新恢复验收**：分别测运行中、已完成、部分成功/失败任务刷新；核实浏览器存储被禁用、task/run 已被清理、暂时断网时的表现。
5. **用户可读性验收**：确认真实响应中 quality.issues、市场指标、因子、策略决策与执行评估字段和 API 契约一致；没有返回的字段应清晰显示，不推断含义。
6. **后续历史功能**：当前同一浏览器保留最近 10 条 task/run 引用，可点选恢复，但只保存引用信息、不保存完整报告，也不能跨浏览器同步。跨设备服务端历史列表与同股历史对比仍需后端分页历史接口及明确的时点/字段比较口径。
7. **策略语义核对**：当前一键入口使用现有 post_market_scan 类型。部署验收时仍需确认现有 Go Worker 对单个 symbols 的业务语义符合预期；前端不会假设该任务等同于完整自然语言研报。

## 需要注意的边界

- 分析 API 当前按六位 A 股代码校验；行情 API 可能需要交易所后缀。因此只在分析请求组装阶段转换代码，不能全局删除后缀。
- 任务 succeeded 与 Run completed 是不同层级的状态；页面按各自 API 状态呈现，不相互替换。
- RunResult 当前是结构化量化工作流结果，不是完整的自然语言 AI 研报；页面不会把字段拼装成未经验证的买卖建议。
- 不从空字段推断 0、通过、已成交、无风险或数据完整；不生成后端未返回的评分、置信度、目标价或止损价。
- localStorage 不是安全保险箱，也不是服务器历史；仅保存最近任务引用信息，不保存 API 返回的完整研报。共享浏览器会话仍需考虑本地存储访问风险。
- 公网部署前必须使用 HTTPS 和外部身份认证/访问限制；行情 API Key 不等于用户身份认证。
- CI 成功只证明静态检查、单元测试和镜像构建通过；不表示后端在线、API Key 有效或 NAS/1Panel 环境已联调。

## 验收结果

- GitHub Actions：已通过五轮检查，最新一轮 [#39896321052](https://github.com/Kasign/stock_data_provider/actions/runs/39896321052) 覆盖统一错误诊断与缺失值处理。
- NAS / 1Panel 真实联调：待执行。
- 三类真实数据响应：待执行。


## 下一轮前端错误与缺失数据修正

- 统一 HTTP 错误展示：401 不再一律解释为行情 API Key 问题；显示 HTTP 状态、服务端错误码和 request ID（若返回）。
- 分析任务创建/输入校验失败改为任务页面内联提示，避免使用阻断式 alert。
- 任务未返回 completed_stages 时显示“未报告”，不再把缺失值显示为 0。
- 对应回归测试已加入；本轮提交 CI 结果待回填。
