# WebUI 统一诊断接入计划

- 目标分支：`feat/webui-market-data-console`
- 当前基线：`d1ad337d4ef8afd6b1242eaaea8869102bf81d65`
- 更新日期：2026-11-03

> 本文只保留诊断专项字段、展示和联调约束；当前是否推进以 [WebUI 整体规划](./整体规划.md) 为准。
- 范围：仅修改当前 WebUI 分支的 `web/**` 与 `doc/**`；不创建新分支，不修改 main，不修改 Go Provider、服务端路由、数据库、OpenAPI 或部署配置。

## 1. 目标与边界

数据侧负责诊断判断和证据生成；WebUI 负责诊断参数交互、真实响应校验、状态表达、证据展示和历史查看。WebUI 不复制字段质量规则，不根据行情值自行判定 Provider 质量，也不使用模拟数据作为真实运行状态。

## 2. 实施任务

| 阶段 | 任务 | 状态 |
|---|---|---|
| 1 | 核对统一主动探测接口、响应模型、页面调用链和历史接口 | 已完成 |
| 2 | 展示字段校验计数、字段级问题、截断信息 | 已完成，自动化通过 |
| 3 | 展示市场宽度四类覆盖率维度，严格使用服务端 ratio | 已完成，自动化通过 |
| 4 | 区分源端时间、采集时间、检查时间；Quality 与 Freshness 分离；跳过状态不误报 | 已完成，自动化通过 |
| 5 | 同步功能说明、API 映射、提测清单和本计划 | 已完成 |
| 6 | WebUI 单测、语法检查、浏览器专项及集中回归 | 已完成：356/356 单测、语法、镜像构建及 Chromium 专项通过 |
| 7 | 部署后对照真实探测 JSON 验证字段、覆盖率和历史持久化 | 暂缓，等待数据侧接口开发与验证完成 |

## 3. 数据契约说明

当前数据侧 Go `ProviderProbeItem` 已包含：

- `validation_total_count` / `validation_error_count` / `validation_warning_count` / `validation_info_count`
- `validation_truncated` / `validation_issues`
- `coverage_dimensions`

字段问题包括解析、标准化、校验阶段以及标准字段、Provider 原始字段、规则编码、期望、实际类型、脱敏实际值、严重级别和消息。市场宽度覆盖维度为 quote_response、price_classification、quote_timestamp、amount。

当前 `services/market-data/openapi.yaml` 的 `ProviderProbeItem` Schema 尚未声明这些字段，属于数据侧文档缺口。本分支只记录，不修改服务端文件。若后续 Go JSON 响应与 OpenAPI 不一致，以数据侧补齐契约为准；WebUI 不猜测不存在的字段。

## 4. 展示约束

1. 字段校验计数使用服务端计数；明细最多展示 30 条，服务端截断必须提示。
2. 实际值只展示服务端提供的脱敏摘要，并使用 textContent 输出。
3. 覆盖率只展示服务端 ratio；不根据返回数、有效数或总数在浏览器端补算。
4. 不同覆盖维度独立展示，不把总体有效证券覆盖率当作价格/时间/成交额覆盖率。
5. provider_time、fetched_at、checked_at 分别表示源端时间、采集时间、检查时间，缺失时不互相代替。
6. Quality 与 Freshness 独立；skipped、not_configured、not_supported 不得映射为健康。
7. 历史记录复用同一渲染逻辑；历史读取失败不得清空或覆盖本次探测结果。

## 5. 验收标准

- 有字段问题、无字段问题、明细截断、仅有计数无明细均能正确显示。
- 四种覆盖维度分别显示正确 ratio 和分母证据；ratio 缺失/非法时显示未知。
- 源端时间、采集时间和检查时间可独立缺失，不发生时间字段替代。
- Quality=degraded 与 Freshness=stale 可以同时显示，不互相覆盖。
- skipped / not_configured / not_supported 显示非成功状态。
- 当前探测和历史探测使用相同字段展示规则。
- 单元测试、语法检查、Chromium 专项通过；真实部署验收另行记录，不以模拟 API 测试代替。


## 6. 本批自动化结果（2026-11-03）

- WebUI Checks：356/356 单元测试通过，JS 语法检查、Nginx 镜像构建通过。运行记录：[41592118649](https://github.com/Kasign/stock_data_provider/actions/runs/41592118649)。
- Chromium Browser Smoke：响应式布局、Run Flow、Run Flow 结果恢复、个股研究交互/数据质量、Provider 主动探测专项全部通过。运行记录：[41592118635](https://github.com/Kasign/stock_data_provider/actions/runs/41592118635)。
- 上述自动化运行对应功能代码提交 `d5fc8665ab2e8d5d3a17a68bb7bb67bc9cf88d61`；后续仅文档索引/映射和验收状态更新，不改变本批 WebUI 代码。
- 真实部署验收仍待执行，不能由模拟 API 的 Chromium 测试替代。


## 7. 当前开发决策（2026-11-03）

统一诊断任务当前不继续扩展 WebUI 功能。原因是数据侧接口尚未完成全面验证，现阶段仅凭模拟 API 或部分真实回包无法确认所有字段、枚举、覆盖率分母和历史持久化行为稳定。

恢复条件：
1. 数据侧完成 ProviderProbeItem 响应模型与 OpenAPI 契约同步。
2. 数据侧使用真实 Provider 验证接口连通性、字段校验、覆盖率及跳过/未配置状态。
3. 数据侧验证诊断历史持久化、查询及降级行为。
4. WebUI 对照最终真实响应样例进行联调；只修正实际发现的兼容问题，不复制数据侧质量规则。

等待期间保留已完成的前端展示，不修改 Go、Provider、数据库、服务端路由或 OpenAPI；诊断相关 UI 深化标记为暂缓，不阻塞其他独立 WebUI 任务。
