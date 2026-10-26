# WebUI 实施进度

更新时间：2026-10-23  
目标分支：`feat/webui-market-data-console`  
范围：只修改 `web/` 前端、前端测试与相关文档；不改 Go API、数据源、数据库或任务契约。

## 当前分支基线

- 当前处理基线：`36dc8bcae53598104e97fa81f1ecc88ad2e58b2b`。
- 基线 GitHub Actions [#39896407950](https://github.com/Kasign/stock_data_provider/actions/runs/39896407950) 已通过：JavaScript 语法检查、19 项 Node 测试和 Nginx Docker 镜像构建。
- 本轮新增 API 调用契约清单与 3 项边界测试；新提交的最终 CI 应通过 PR 的 Checks 页面确认。

## 已实现的前端功能

| 功能 | 当前状态 | 说明 |
|---|---|---|
| 行情工作台 | 已实现，待真实 API 验收 | 服务就绪检查、Provider 状态、实时行情、日 K 曲线/表格、质量状态及警告。 |
| 发起单股分析 | 已实现，待真实业务语义验收 | 从当前行情标的入口提交现有 `post_market_scan` 任务；分析请求将交易所代码规范化为六位 A 股代码。 |
| 任务进度与事件 | 已实现，待真实 API 验收 | 查询 task 状态、按 `after_sequence` 增量读取阶段事件；不伪造进度百分比。 |
| 结构化 RunResult | 已实现，待真实回包验收 | 展示市场状态、质量、因子、候选信号、可交易性、风险、计划/拒绝原因、执行评估、失败与警告。 |
| 缺失值与异常显示 | 已实现 | 缺失字段显示为未报告；错误信息尽可能包含 HTTP 状态、服务端错误码和请求 ID。 |
| 最近分析与刷新恢复 | 已实现，待浏览器联调 | 当前浏览器最多保存 10 条 task/run 引用，通过既有接口恢复；不缓存整份报告，也不是服务端历史。 |
| 报告导出 | 已实现 | 支持原始 JSON 与结构化 Markdown 下载，缺失数据会明确标记，不生成虚构评分或交易结论。 |
| 连接和密钥设置 | 已实现 | 配置同源代理路径；行情 API Key 存于浏览器 localStorage，只附加在行情数据接口请求中。 |

## API 协议状态

- 当前 WebUI 使用的 API 路径与行为已整理在 [API 集成契约](./API_CONTRACT.md)。
- 分析异步任务已有 OpenAPI 文档；行情端点与 Run 结果端点的完整统一契约仍有文档覆盖缺口。
- 此 WebUI 分支不修改 Go 接口、不新增后端端点、不改变任务 payload。若发现不一致，应对照 Go 路由、领域类型和后端测试另行处理，不能让前端猜测或单方面重定义协议。

## 本轮新增与自动化验证

- 为行情健康检查、分析健康检查、Provider 清单和日 K 请求增加调用契约测试，覆盖路径与行情 API Key 隔离。
- 增加非 JSON 反向代理错误测试，检查 HTTP 状态、请求 ID 与错误文本长度限制。
- 增加结构化错误 envelope 中 request ID 优先级测试。
- 本轮修改完成后，以对应分支最新的 GitHub Actions 为准回填测试数量与构建结果。

## 尚待验收

1. **NAS / 1Panel 真实联调**：确认 `stock_analysis_web` 在 `1panel-network` 内能访问 `market-data:8091` 和 `stock-analysis-go:8092`，并验证 API Key 传递与代理前缀剥离。
2. **真实数据场景**：逐项对照页面与原始 JSON，至少包含完整数据、可选数据缺失/质量降级、后端故障三种场景。
3. **真实任务语义**：确认单个股票代码配合 `post_market_scan` 的行为与用户期望一致；结构化 RunResult 不能冒充完整自然语言研报。
4. **刷新与失效任务**：验证运行中、已完成、部分成功/失败、后端记录已清理、暂时断网与浏览器存储不可用时的表现。
5. **公网安全**：当前 Go 分析服务不内置用户认证；公网使用前必须在 OpenResty/身份认证层限制访问并启用 HTTPS。

## 仍未实现（不应通过前端猜测补齐）

- 服务端分页历史列表、跨设备自选股和同股历史报告对比，需要后端 API 及明确比较口径。
- 行业/题材、市场广度、新闻、财务、资金流、筹码等信息，只有后端实际返回且质量可判定时才展示。
- 条件选股、Agent、回测、告警、持仓管理等不属于当前单股分析闭环的前置工作。


## 任务恢复与本地存储边界回归

- 新增任务恢复源码契约测试：损坏/无效引用被拒绝，激活记录会停止旧轮询、重置事件游标并从服务端重新拉取状态；异步请求响应必须验证当前 task/generation，避免旧任务覆盖新任务。
- 新增本地历史安全测试：最近任务存储只保留 task/run 引用白名单字段，不会把报告正文、RunResult 或 API Key 写入 localStorage。
- 上一批 [CI #39897407071](https://github.com/Kasign/stock_data_provider/actions/runs/39897407071) 已通过 22 项测试；本批新增 2 项测试。当前分支最新 CI 请以 [PR #2 Checks](https://github.com/Kasign/stock_data_provider/pull/2) 为准。
- 这些是 Node 层的回归测试，不替代 NAS/1Panel 下的真实浏览器刷新、断网和过期 task/run 验收。


## 任务状态错误与 Run 结果重试

- 任务状态轮询现区分暂时性网络/服务错误与不可重试的普通 4xx。对 401/403/404 等配置/资源问题停止定时轮询并提示手动重试入口，避免无效任务 ID 导致持续请求。
- Run 结果读取对暂时性错误最多自动重试 3 次（1 秒、2.5 秒、5 秒）；切换任务或发起新读取时，旧重试循环失效。
- 新增可重试错误分类单元测试与任务轮询/Run 结果恢复契约测试。
- 以上是前端错误恢复策略，不会重新创建任务，也不改变 Go API 的错误协议或业务状态。


## 下一项 WebUI 体验改进

- 结构化分析结果之前大量使用 9–10px 字号，标签、内容与操作入口在较长报告中不易阅读。
- 当前批次提高摘要指标、结果分区、字段标签、字段内容、警告和导出按钮的字号/间距，并保留 600px 以下单列自适应布局。
- 增加 CSS 契约回归测试，防止结果正文再次被压缩到过小字号；该测试只验证样式源码，仍需后续浏览器截图/真实设备验收。
- 本轮最新 CI 结果待回填。


## 本批 Go RunResult 场景 fixture 与报告导出回归

- 新增 `tests/fixtures/run-results.mjs`，按 `internal/analysis/domain/types.go` 的 JSON 字段构造确定性合成数据；所有数字、信号、计划和失败消息只用于测试，不代表真实行情、真实分析结果或交易建议。
- 报告导出测试扩展为六类结果：完整成功（含因子/信号/计划/执行评估）、成功但无数据、部分数据、数据质量阻断、请求拒绝、运行失败。
- 检查质量门禁、策略资格/阻断原因、因子缺失原因、规则、计划字段、执行数量，以及失败代码/阶段/消息是否按服务端返回呈现。
- 现有 API/错误分类测试还覆盖 409 `run_not_finished`、请求 ID、网络故障和暂时性/永久性 HTTP 错误；这类单元测试不代替真实 Go 服务联调，也不代表 DOM/浏览器测试已完整。
- 本提交对应的 GitHub Actions 结果待回填；本轮 CI 通过后再将相应完成项标记为已验证。


## 本地自选股与快捷访问（本批待 CI）

- 在行情工作台加入“加入自选”入口，以及本地自选列表；每项支持编辑自定义名称、查询行情、直接发起单股分析和移除。
- 自选列表上限 50，只持久化代码、名称和更新时间；不存 API Key、完整 RunResult 或报告正文，不增加 Go API/数据库字段。
- 新增独立纯 JS 模块 `src/watchlist.js` 与测试，覆盖格式校验、重复标的去重、别名保留/编辑、最多 50 条、损坏存储、存储异常及敏感/非白名单字段剔除。
- 添加快捷查询时会先检查当前行情查询是否仍在运行，避免误改输入框后被旧请求覆盖；发起分析沿用既有 Go 单股任务入口。
- 已扩展静态 DOM 契约检查与响应式样式断言。当前提交的 Node 测试、语法检查和 Nginx 构建结果尚待 GitHub Actions 确认。
- 明确边界：自选仅限当前浏览器，不支持跨设备同步；股票代码/名称仍以用户输入和已返回行情为准，不在前端臆造代码补全。


## 日 K 图表升级（本批待 CI）

- 依据现有 `services/market-data/internal/model.DailyBar` 与 bars HTTP 响应字段实现 OHLC K 线/成交量视图、收盘线视图切换；未新增服务端接口。
- 图表按日期升序稳定排序；缺失/null/非数值字段不作为 0 画图，K 线 OHLC 相互矛盾时不绘制该根 K 线。
- K 线原生 SVG 悬停提示显示日期、开高低收、成交量/成交额、涨跌幅、数据源、复权口径、质量状态/原因和可能的回退来源；成交量未提供时明确显示“未报告”。
- 接入已有 bars API 的可选 `start`/`end` 参数，并按 Asia/Shanghai 当日进行前端校验；保留 `limit` 作为结果数量上限，复权口径依旧由原选项和后端返回值决定。
- 修复日 K 请求失败后旧股票图表可能继续显示的问题，失败时清空旧 K 线并展示诊断提示。
- 新增独立 SVG chart 单元测试、日期参数 API 路由测试、UI DOM/CSS 契约测试。本批 GitHub Actions 结果待确认；未将 CI 视为真实 NAS/1Panel 联调。


## 日 K 图表升级 — CI 验证结果

- 提交：[`4008203b`](https://github.com/Kasign/stock_data_provider/commit/4008203b056ca11038093b855b547ea6b312985a)。
- GitHub Actions：[WebUI checks #39906212815](https://github.com/Kasign/stock_data_provider/actions/runs/39906212815) 成功：42 项 Node 测试通过（0 失败），JavaScript 语法检查通过，Nginx 镜像构建通过。
- 收盘线/蜡烛图切换、OHLC 与成交量柱、悬停详情、日期范围请求参数、缺失字段与无效 OHLC 边界测试均已覆盖。
- 本验证只覆盖前端模块契约及构建；尚未对 NAS/1Panel 真实行情做字段、复权、访问控制和浏览器交互验收。
- 下一项依赖后端能力：分析历史列表 API。前端在 API 真正落地前不会伪造服务端历史、回放或跨设备同步。


## 个股补充研究面板（本批待 CI）

- 接入仓库现有的 `GET /api/v1/stocks/{symbol}/chip-distribution` 和 `GET /api/v1/stocks/{symbol}/capital-flow`，不新增、不修改后端接口。
- 面板采用手动加载，不给每次行情查询额外增加两个请求；两个端点通过 `Promise.allSettled` 独立呈现，任一个失败时保留另一个成功结果。
- 筹码卡展示平均成本、获利比例、90%/70% 集中度、数据日期、来源/版本、获取时间、质量状态与质量原因。比例仅按后端的 [0,1] 约定乘 100 显示。
- 资金流卡展示主力净流入及 5/10 日合计、有效观测数、来源/版本、获取时间、质量状态与质量原因；未提供的聚合值保持为“—”，不补造 0。
- 切换查询标的时清除旧标的补充数据；通过 generation 检查避免较早返回的请求覆盖新标的状态。
- 新增 API 路由/鉴权测试及 UI 契约测试。GitHub Actions [#39910050023](https://github.com/Kasign/stock_data_provider/actions/runs/39910050023) 通过：44 项 Node 测试通过（0 失败），JS 语法检查和 Nginx 镜像构建均通过。状态/错误仍由各接口独立报告，不构建“主力买入/卖出”这类未经后端定义的结论。真实补充 Provider 的可用性和字段值仍需部署验收。


## 最近分析本地历史管理 — CI 验证结果

- 最近分析区域增加关键词检索，覆盖股票代码、task ID、run ID、任务类型、状态与更新时间；限定为当前浏览器已保存的最近 10 条引用。
- 每条记录可单独移除；可清空本地列表，操作前明确说明不会删除服务端任务或报告。
- 同步清理旧的最近一次任务指针，防止刷新恢复逻辑将已移除/已清空的任务重新迁回列表。
- 若移除/清空当前页面正在查看的任务，只停止本地引用的后续持久化，不取消任务、不影响当前页继续查看服务端响应。
- 增加本地存储模块测试与 UI 源码契约测试。GitHub Actions [WebUI checks #39911788624](https://github.com/Kasign/stock_data_provider/actions/runs/39911788624) 已通过：48 项 Node 测试全部通过（0 失败），JavaScript 语法检查和 Nginx 镜像构建成功。浏览器存储被禁用时仍保持页面可用，但无法承诺跨刷新保存清理结果。


- 代码提交：[`230ac4f1`](https://github.com/Kasign/stock_data_provider/commit/230ac4f16ffa4c48fe24692ee947632ca5dcc027)。CI 只验证源码和镜像构建；搜索、清理与刷新恢复的真实浏览器交互仍需部署后验收。


## 全局字号可读性与 API/持久化边界说明 — CI 验证结果

- 全局字体基线由 13px 提升至 15px；原来 9–10px 的辅助文字提升至至少 12px，原来 11–12px 的标签/次级正文提升到 13px。表格正文、表单控件、报告字段与原始 JSON 查看区分别设定可读字号。
- 调整小屏下搜索表单、指标卡和报告内容间距，避免字号放大后控件挤压；保留图表、状态标识和密集数据区的独立布局。
- 在连接设置中加入“API 和数据分别存在哪里”的说明：WebUI 不直连数据库；浏览器只保存 API Key、自选股和最近任务引用；Go 分析服务将任务/阶段事件/结果/快照持久化到 MariaDB，Redis 是缓存；行情数据持久化由行情服务端点决定。
- README 与 API 契约同步补充数据流及持久化边界，避免把 WebUI localStorage 误解成服务端分析历史。
- 新增字号基线与设置区内容的 UI 契约测试。GitHub Actions [WebUI checks #39927566854](https://github.com/Kasign/stock_data_provider/actions/runs/39927566854) 成功：49 项 Node 测试通过（0 失败），JavaScript 语法检查和 Nginx 镜像构建通过。


- 代码提交：[`ddb580b2`](https://github.com/Kasign/stock_data_provider/commit/ddb580b22ee99753cbab2556caaabc31e3308030)。实际浏览器排版与 NAS/1Panel 后端联调仍需部署后验收。


## 分析任务重复提交保护 — CI 验证结果

- 增加 `state.isCreatingTask` 锁，在表单事件处理器入口立即设置，阻止快速重复触发创建请求。
- 输入格式校验失败会释放锁；网络成功/失败统一在 `finally` 释放并恢复提交按钮，避免卡死。
- 保留并测试可选 `Idempotency-Key` 请求头透传。前端锁只能防止当前页面内的并发重复请求，跨网络重试仍需调用方复用幂等键。
- 新增任务创建防重复提交源码契约测试。GitHub Actions [WebUI checks #39928330146](https://github.com/Kasign/stock_data_provider/actions/runs/39928330146) 成功：50 项 Node 测试通过（0 失败），JavaScript 语法检查与 Nginx 镜像构建成功。


- 代码提交：[`c87c4dba`](https://github.com/Kasign/stock_data_provider/commit/c87c4dba75b04595d5f0a899ea4a0dbde6a33a4b)。此锁只覆盖当前页面的并发提交；网络中断后用户应复用幂等键确认服务端是否已接收任务。


## API Key 配置说明与样式 Review — CI 验证结果

- 明确 `MARKET_DATA_API_KEY` 由部署管理员生成，不是行情 Provider 的个人 Key，也不是 WebUI 自动创建的令牌；推荐在部署服务器执行 `openssl rand -hex 32` 生成。
- Go 行情服务和 Go 分析服务共用部署环境中的 `MARKET_DATA_API_KEY`；WebUI 只把相同值保存在当前浏览器 `localStorage`，并在受保护的行情请求中通过 `X-API-Key` 发送，不会写入后端 `.env` 或数据库。
- Review 清理了设置区重复的 CSS 规则；连接设置页和 README/API 契约已补充密钥归属、生成方式与存储边界。
- GitHub Actions [WebUI checks #39929408226](https://github.com/Kasign/stock_data_provider/actions/runs/39929408226) 通过：51 项 Node 测试全部通过（0 失败），JS 语法检查与 Nginx 镜像构建成功。
- 代码提交：[`07797f2f`](https://github.com/Kasign/stock_data_provider/commit/07797f2f67486e1d9f552632c1fd467357617ff3)。


## API Key 输入体验与 CSS 回归 Review（本批）

- 行情 API Key 默认以密码样式遮蔽；用户主动点击“显示”后才呈现明文，关闭设置对话框（取消、保存或 Esc）后自动恢复遮蔽。按钮包含稳定的无障碍名称、`aria-controls` 与 `aria-pressed` 状态，键盘焦点有可见边框。
- 修正品牌副标题在字号全局覆盖中被后置规则降回 11px 的问题，保持至少 12px。
- 修正字号回归测试的误判方式：测试现在读取指定选择器最后一条 CSS 声明，避免只匹配前面的旧规则而无法检查实际生效的字号。
- GitHub Actions [WebUI checks #39933160719](https://github.com/Kasign/stock_data_provider/actions/runs/39933160719) 通过：53 项 Node 测试全部通过（0 失败），JavaScript 语法检查和 Nginx 镜像构建通过。此改动未触及 API、数据源与服务端密钥存储；真实浏览器的键盘/窗口关闭行为仍待部署验收。

- 安全 Review 清理任务按钮恢复文案时残留的固定 `innerHTML` 赋值，改用 DOM 节点；增加回归测试，禁止页面将 API/浏览器数据通过 HTML 字符串插入。


## Markdown 导出与 API 错误诊断边界（本批实现）

- 报告顶部新增结果摘要；对缺失字段不推算记录数，也不生成交易判断。
- Markdown 导出现在对服务端普通字符串中的换行和 Markdown 特殊字符做转义；对象和数组使用行内 JSON 代码格式，减少导出内容打乱文档结构的风险。
- RunResult 集合分别呈现“字段缺失或 null”“明确空数组”“错误的非数组类型”，不会把不同数据状态统一成“未返回”。
- 错误响应的消息、错误码、请求 ID 分别限制长度；补充大体积结构化错误消息和 JSON 字符串响应测试。
- 验证类型：Node 单元测试使用固定合成 RunResult 和模拟 fetch 响应；这只验证前端边界行为。真实字段比对、真实 API 错误协议、刷新恢复和浏览器布局仍须 NAS/1Panel 环境验收。
- 基线回查：开发分支在本批开始前为 `b22f00f4a6f821b4edccb0007e16fef35afe8316`，对应 WebUI checks [#39933160719](https://github.com/Kasign/stock_data_provider/actions/runs/39933160719) 成功。本批代码与测试对应的 [WebUI checks #39934244200](https://github.com/Kasign/stock_data_provider/actions/runs/39934244200) 已成功：56 项 Node 测试全通过，JavaScript 语法检查及 Nginx 镜像构建通过。目标分支后续进展以 PR 检查结果为准；本 CI 不是 NAS/1Panel 真实环境验收。PR 仍保持独立开放，未执行合并。



## 主题切换与移动端体验（本批）

- 新增深色/浅色可切换主题，主题模块独立于数据 API；偏好只写入浏览器主题键，localStorage 不可用时仍能应用本次选择。
- 轻主题调整常用页面表面、输入框、指标卡、日 K 表格、结构化报告、质量状态与设置弹窗的颜色对比度。
- 手机宽度下侧边栏改为抽屉导航：支持打开/关闭、遮罩关闭、Esc 关闭和导航后自动关闭，不再占用固定的 67px 内容宽度。
- 新增 `tests/theme.test.mjs`，覆盖主题值校验、持久化、存储读写异常和 root theme 应用；UI 契约测试增加主题和移动抽屉检查。
- 本功能不增加数据端依赖；自动化验证只证明源码/模块行为，未代替 iPhone/Android 浏览器的真实交互验收。
- 管理员密码和真实服务端 `.env` 导入/导出暂不实现：当前服务没有相应契约，必须先增加后端认证/配置管理能力，不可使用 localStorage 做密码保护或声称浏览器导入会修改 NAS 文件。

- 对应 CI：[WebUI checks #39936585174](https://github.com/Kasign/stock_data_provider/actions/runs/39936585174) 成功：62 项 Node 测试通过（0 失败），JavaScript 语法检查和 Nginx 镜像构建通过。主题/移动导航真实设备体验仍待 NAS 部署后人工验收。


## WebUI 功能扩展规划与接力状态（2026-10-23）

- 已核对参考仓库 daily_stock_analysis 的 Home、Settings、Chat、StockScreening、Alerts、RunFlow 和相关 API 模块，比较结论见 web/PRODUCT_BACKLOG.md。
- 已建立功能任务 Issue：#3 全局布局、#4 任务运行流、#5 设置中心、#7 通知中心、#8 问股、#9 选股、#10 告警。
- 当前 WebUI 分支基线为 37b5b6cc1c94d6e43d52cf67779110b6ddc79a66；数据分支为 7e47aec66cb000cf403ed0cfbbcbadfe655467a3；PR #2 保持 Open/未合并。
- WebUI CI 39938663988、Go test suite 39938228769、Live Provider probe 39938228772 均已核实成功；这些结果不是 NAS/1Panel 真实联调结果。
- 当前接口可以支撑行情、日 K、Provider 清单、单个异步 Task、StageEvent 和 RunResult。通用配置管理、通知收件箱、Agent Chat、选股引擎和告警后端接口未核实到，不允许假装其已可用。
- 下一步建议从 Issue #3 做全局布局/导航审查，随后先做现有 API 可以支撑的任务详情/运行流，再做设置/通知 UI，并按产品清单推进问股、选股、告警。
- 新聊天恢复源：web/HANDOFF.md。每个批次结束时更新此文件及 PRODUCT_BACKLOG.md 的执行状态；不要强推、不改写旧历史、不合并 PR #2。


## Issue #3 — 全局布局、导航与视觉骨架（自动化验证通过，浏览器视觉验收待完成）
- 从重新核实的 WebUI 提交 `f8c8921764a3ccfe9531c0565a8844e0e9c70aba` 开始实施；未按旧 HANDOFF 里的过期 HEAD 继续工作。
- 建立七项导航（行情总览、任务与运行流、通知、问股、选股、告警、设置），通过 URL hash 支持直接打开页面与浏览器前进/后退；活动导航同步 `aria-current`。
- 新增通知、问股、选股、告警、设置的 UI 骨架；缺少后端 API 时清楚展示待接入状态，禁用会造成误解的提交操作，不展示模拟通知、模型回复、候选或告警历史。
- 设置页区分本地主题偏好、浏览器连接配置与服务端配置；主题继续走现有主题模块，API 路径/行情 Key 仍通过原连接设置弹窗处理，不声称写入服务器。
- 为新页面补充统一状态徽标、说明面板、空态、表单栅格和响应式样式。保留原生 HTML/CSS/JavaScript ES Modules；未修改 Go、Provider、分析 Runner 或 Python bridge。
- 自动化覆盖：新增导航/hash/可访问活动态、缺少接口时不生成假数据、响应式样式契约测试。GitHub Actions [WebUI checks #39941562678](https://github.com/Kasign/stock_data_provider/actions/runs/39941562678) 成功，JavaScript 语法检查、Node 测试与 Nginx 镜像构建全部通过。
- 真实视觉验收：本环境无法运行实际浏览器截图；移动端/桌面截图 review 尚未完成。NAS/Docker/OpenResty 端到端联调未执行。


## Issue #4 — Task 详情抽屉与真实运行流时间线（Node/CI 验证通过，真实环境待验收）
- 最近任务仍仅来源于浏览器内 task/run 引用；点击某条引用会切换到任务页、通过现有 API 恢复 Task 和事件，再打开右侧运行流抽屉。新建任务收到真实服务端 Task ID 后自动打开抽屉；抽屉可通过关闭按钮、遮罩或 Escape 关闭，关闭不停止服务端任务/本地轮询。
- 抽屉分别显示 Task 状态与 Run 状态。Task 已返回的 `created_at` / `started_at` / `finished_at`、Task ID、Run ID、标的、阶段与错误码直接取服务端字段；缺失值不按客户端时钟补造。
- 阶段事件按现有 sequence 排序，并沿用 `after_sequence` 续读；只在响应真实携带 `duration_ms` 或 `error_code` 时展示对应信息。没有 DAG API，不显示假进度百分比或推算耗时。
- 更新 `web/API_CONTRACT.md`，只记录目前已有的 Task/StageEvent/RunResult 路由与展示边界，没有新增后端契约。
- Review 顺便清理了 Issue #3 导航区重复的“工作台”分组标题。
- 自动化验证：WebUI checks [#39942477418](https://github.com/Kasign/stock_data_provider/actions/runs/39942477418) 对最终功能提交 `23879eca559a2b9a197d34a7a2fd437f53691894` 成功：68/68 项 Node 测试通过，JavaScript 语法检查和 Nginx 镜像构建均通过。先前 #39942197604 与 #39942345586 的失败是两条新增契约断言分别错误定位游标参数文件、假定排序回调变量名；均已修复并重新验收。NAS/Docker/OpenResty 联调与浏览器截图未执行。


## Issue #5 — 分组设置中心与配置边界（CI 通过，实机验收待完成）
- 将设置页拆为九个有可访问导航的分组：显示偏好、连接与服务、基础设置、数据源、AI 与问股、通知渠道、任务调度、选股默认项、系统安全与备份；小屏下分组导航转为两列/三列布局。
- 连接与服务新增真实健康检查按钮，调用现有行情/分析 health API 并展示两个独立结果。数据源快捷入口导航到已有的真实 Provider 清单，不宣称服务端配置可编辑。
- 连接配置实现 dirty state、同源路径验证、保存错误反馈、取消/关闭/Escape 时未保存变更确认，以及清除本地连接配置的单独确认。打开设置时不回显已存 API Key；留空保留当前值，输入新值才替换。
- 新增 `web/src/settings-draft.js` 和 `web/tests/settings-draft.test.mjs`，覆盖变更判断、留空保留/替换 Key、非法远程路径拒绝；UI 契约测试覆盖九个分组、健康检查、离开保护与服务端禁用态。
- 服务端基础设置、AI 模型、通知、调度、选股、密码/服务器 .env/备份没有对应 API，因此相关控件明确禁用，不写入 localStorage，不产生假成功反馈。
- 未修改 Go、Provider、分析任务或真实后端路由。真实浏览器截图、键盘/焦点走查及 NAS/Docker/OpenResty 联调未执行。


- 首轮 WebUI checks [#40256527920](https://github.com/Kasign/stock_data_provider/actions/runs/40256527920) 因三项旧源码契约断言失败（69/72 通过）未执行镜像构建；已修正文案/回调形式相关断言并复验成功。最终 WebUI checks [#40256618763](https://github.com/Kasign/stock_data_provider/actions/runs/40256618763) 全部通过：72/72 项 Node 测试、JavaScript 语法检查、Nginx 镜像构建均成功。浏览器真实交互与 NAS/Docker/OpenResty 联调仍未执行。


## Issue #7 — 通知中心 UI 与五态模型（本批已提交，CI 待回填）
- 通知页面补齐分类/状态/关键词筛选、刷新入口、通知列表和详情区域；因为通知 API 不存在，筛选、刷新、标记已读/未读均禁用，不展示静态通知、计数或投递成功记录。
- 新增 loading / empty / unavailable / error / loaded 五态的纯函数状态解析与页面呈现框架。当前初始化明确走 unavailable；空数组只有在未来 API 真正返回时才会解释为 empty。
- 顶栏只保留通知入口，不显示未读数。通知收件箱与设置中的渠道配置、告警中的通知投递尝试分离。
- 新增 `web/src/notification-state.js`、`web/tests/notification-state.test.mjs` 和 UI 契约测试；语法检查包含新模块。后端 API 缺口未改变，也未添加模拟请求或 fixtures 到生产页面。
- 本批 CI 状态待回填；浏览器实际布局及 NAS/Docker 真实通知 API 联调尚未执行。


- Issue #7 首轮 WebUI checks [#40256844942](https://github.com/Kasign/stock_data_provider/actions/runs/40256844942) 中 74/75 项 Node 测试通过，JS 语法检查通过，Nginx 镜像构建因测试失败跳过。唯一失败是沿用旧文案精确匹配“不会展示模拟通知”，而页面文案调整为“不会生成模拟通知”；已放宽为接受这两种等价表达，等待重新验收。
