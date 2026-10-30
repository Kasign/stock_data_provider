# 数据源真实环境验收记录

- 适用分支：`feat/full-go-data-providers-api-switch`
- 当前基线：`9fd604783a497aa8236d17c8961468f24769af80`
- 更新日期：2026-10-30
- 当前状态：尚未在本次阶段一中执行供应商真实请求；本文件先建立统一记录模板，不代表任何 Provider 已通过在线验收。

## 1. 验收状态口径

| 状态 | 说明 |
|---|---|
| 未执行 | 没有真实请求证据 |
| 请求成功待核对 | HTTP/业务请求成功，字段和质量尚未审查 |
| 通过 | 字段、单位、时间、覆盖和质量契约均符合预期 |
| 降级可接受 | 返回 degraded/unknown，原因明确且不会污染可信数据 |
| 不通过 | 返回错误数据、错误单位、错误时间或质量状态丢失 |
| 权限阻塞 | Token、积分、Cookie 或账号权限不足 |
| 限流阻塞 | 上游限频或并发限制阻塞 |
| 暂缓 | 不属于当前国内 A 股验收范围 |

## 2. Provider 验收矩阵

| Provider | 能力 | 配置状态 | 真实请求状态 | 样本/日期 | 结论与证据 |
|---|---|---|---|---|---|
| 东方财富 | quote/daily_bars | 默认启用 | 未执行 | 待补 | 待验收 |
| 东方财富 | fundamentals/valuations | 相关能力需按实际注册核对 | 未执行 | 待补 | 待验收 |
| 东方财富 | breadth/sectors/limit_pools | 默认启用 | 未执行 | 待补 | 待验收 |
| 腾讯 | quote/daily_bars/search | 默认启用 | 未执行 | 待补 | 待验收 |
| 新浪 | quote/daily_bars | 默认启用 | 未执行 | 待补 | 待验收 |
| 网易 | quote/daily_bars | 默认启用 | 未执行 | 待补 | 待验收 |
| Pytdx | quote/daily_bars | 默认启用 | 未执行 | 待补 | 待验收 |
| BaoStock | daily_bars/calendar/fundamentals | 默认启用 | 未执行 | 待补 | 待验收 |
| TickFlow | quote/index/breadth/sectors | 代码实现，部分默认关闭 | 未执行 | 待补 | 待验证权限和覆盖 |
| 同花顺 | quote/daily/index/fundamentals/valuations 等 | 默认关闭 | 未执行 | 待补 | 待验证权限和覆盖 |
| Tushare | stock_basic/calendar/status/fundamentals/valuations 等 | 默认关闭 | 未执行 | 待补 | 待验证 Token 积分权限 |
| 雪球 | hot_rankings | 默认关闭 | 未执行 | 待补 | 待验证 Cookie/限流 |
| 妙想 | chip_distribution/capital_flow | 独立开关 | 未执行 | 待补 | 待验证 API Key/权限 |

表格中的“默认启用”只代表仓库配置，不代表真实上游请求已通过。

## 3. 单次验收记录模板

每次真实请求至少记录：

- 日期、环境、分支和代码提交 SHA；
- Provider、capability、接口/请求参数；
- 脱敏证券代码及交易日；
- HTTP 状态、业务状态、耗时和重试次数；
- 返回记录数、上游总数、去重数、分页终止证据；
- 关键字段存在率、null/zero 分布；
- 价格/成交量/成交额单位和转换依据；
- adjustment_basis、adjustment_method；
- provider_time、report_date、announcement_time、fetched_at；
- quality_status、quality_reasons、fallback 来源；
- 是否出现重复、乱序、越界、异常尺度；
- 结论、失败原因和后续处理。

禁止在记录中写入 API Key、Token、Cookie、账号密码或未经脱敏的个人数据。

## 4. 固定样本建议

至少覆盖：
- 沪市主板、深市主板、创业板、科创板、北交所；
- 正常交易日、停牌日、非交易日；
- 除权除息前后；
- 成交额缺失、成交量缺失、真实零值；
- 上游空响应、分页截断、重复行、超时和限流；
- 财务报告期与公告日不一致、财务修订；
- 新股上市、ST/风险警示、退市和涨跌幅特殊阶段。

## 5. 自动化真实请求入口\n\nTickFlow 已增加独立的 opt-in 真实请求验收测试：\n\n```bash\nMARKET_DATA_LIVE_TICKFLOW_TESTS=1 \\\nMARKET_DATA_TICKFLOW_API_KEY=... \\\ngo test ./internal/provider -run '^TestLiveTickFlowCapabilities

阶段一仅完成代码、配置和文档静态核对，没有发起供应商在线请求。阶段二新增 TickFlow opt-in 真实请求测试入口，但本轮尚未提供 API Key，也未执行在线验收，因此 TickFlow 及其他 Provider 的真实环境验收状态仍保持“未执行”。后续每完成一个 Provider/capability 验收批次，再追加有证据的记录，不允许预填通过状态。
 -count=1 -v\n```\n\n可选配置：`MARKET_DATA_TICKFLOW_API_URL`、`MARKET_DATA_TICKFLOW_FREE_API_URL`；也兼容 `TICKFLOW_API_KEY`、`TICKFLOW_BASE_URL`、`TICKFLOW_FREE_BASE_URL`。测试覆盖单只 A 股实时行情、核心指数行情、全市场宽度和申万一级行业排行；输出只记录能力、耗时、质量状态/原因和行数，不输出 Key 或原始请求 URL。行业排行当前由成分股行情聚合，验收要求质量状态保持 degraded，不得标记为源端完整排名。\n\n该测试默认跳过，不影响普通 CI；必须在具备 TickFlow API Key 且网络可访问的环境显式执行。测试通过只代表该时点样本通过契约，不代表长期可用或可直接启用默认配置。\n\n## 6. 当前记录

阶段一仅完成代码、配置和文档静态核对，没有发起供应商在线请求。因此所有 Provider 的真实环境验收状态均保持“未执行”。后续每完成一个 Provider/capability 验收批次，再追加有证据的记录，不允许预填通过状态。
