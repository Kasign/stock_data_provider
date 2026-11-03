# WebUI 统一诊断接入计划

- 目标分支：`feat/webui-market-data-console`
- 当前基线：`d1ad337d4ef8afd6b1242eaaea8869102bf81d65`
- 更新日期：2026-11-03
- 范围：仅修改当前 WebUI 分支的 `web/**` 与 `doc/**`；不创建新分支，不修改 main，不修改 Go Provider、服务端路由、数据库、OpenAPI 或部署配置。

## 1. 目标与边界

数据侧负责诊断判断和证据生成；WebUI 负责诊断参数交互、真实响应校验、状态表达、证据展示和历史查看。WebUI 不复制字段质量规则，不根据行情值自行判定 Provider 质量，也不使用模拟数据作为真实运行状态。

## 2. 实施任务

| 阶段 | 任务 | 状态 |
|---|---|---|
| 1 | 核对统一主动探测接口、响应模型、页面调用链和历史接口 | 已完成 |
| 2 | 展示字段校验计数、字段级问题、截断信息 | 本次实现，待自动化验收 |
| 3 | 展示市场宽度四类覆盖率维度，严格使用服务端 ratio | 本次实现，待自动化验收 |
| 4 | 区分源端时间、采集时间、检查时间；Quality 与 Freshness 分离；跳过状态不误报 | 本次核对并补齐采集时间，待自动化验收 |
| 5 | 同步功能说明、API 映射、提测清单和本计划 | 已完成 |
| 6 | WebUI 单测、语法检查、浏览器专项及集中回归 | 待执行 |
| 7 | 部署后对照真实探测 JSON 验证字段、覆盖率和历史持久化 | 待部署验收 |

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
