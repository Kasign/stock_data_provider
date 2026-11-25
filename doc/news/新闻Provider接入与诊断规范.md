# News Provider 接入与诊断规范

更新时间：2026-11-25  
适用分支：`repair_all`

本文是当前 Provider 接入规则。旧版 N1/N2/N3 评审、历史迁移方案和已废弃协议草案不再作为实施依据。

## 1. Provider 边界

当前 Provider Adapter 集合：

- Bocha
- Tavily
- Brave
- SearXNG（显式配置时启用）

Provider 负责返回搜索候选，不负责创建新闻主体，也不负责 Feed 来源正文采集。

## 2. 调用链

1. Research/Researchflow 调用 News Search Facade。
2. Facade 按固定 Provider 顺序执行已配置 Adapter，汇总每个 Provider 的独立 Outcome。
3. SearchCandidates 默认写入 Researchflow StageOutput，不自动写入正式 News。
4. 研究流程明确选中/确认候选后，调用 `IngestProviderCandidate`。
5. Provider 候选与 RSS/Atom 采集结果共用同一个正式入库事务。
6. Provider Summary 保留为候选来源信息，不冒充发布者 SourceSummary；正式新闻正文以可信 Source/Feed 内容层为准。

## 3. Adapter 与 Platform 复用

- Provider Adapter 只依赖注入的 HTTPDoer，不创建裸 HTTP Client、连接池或通用重试器。
- 生产 HTTP 请求使用 Platform HTTP Client 适配器。
- 每个 Provider Endpoint 独立绑定 DestinationPolicy，不跨 Endpoint 共享授权范围。
- Provider 自身的 API Key、请求参数、响应解析、分页和业务错误映射由 Provider Adapter 负责。
- TLS、DNS/IP 校验、拨号、总 Deadline、响应体大小和通用传输 Fault 由 Platform 提供。
- 不新增 Provider 专属 Scheduler、Queue、Worker、Orchestrator 或跨请求缓存。

## 4. 统一结果与错误语义

每个 Provider Outcome 至少区分：

- `success`：成功返回候选。
- `empty`：请求成功但没有候选。
- `partial`：部分结果可用，存在可定位的部分失败。
- `skipped`：未配置、未启用或缺少凭证。
- `failed`：真实请求或响应处理失败。

稳定错误分类包括认证失败、限流、超时、上游不可用、响应结构无效和配置错误。错误不得包含 API Key、Authorization Header、Cookie、DSN、原始响应正文或带凭证 URL。

技术可重试性与业务是否应该重试分离；不得因 `retryable=true` 无限重试。

## 5. 诊断要求

- Provider 缺少凭证或未启用：`skipped`，不是失败。
- 已配置 Provider 的 DNS/TLS/HTTP/超时/响应解析问题：`failed`，并保留稳定 reason_code。
- 成功但无候选：`empty`，不是数据质量通过。
- 没有真实样本或无法比较：`not_comparable`。
- 诊断记录必须能定位 Provider、Endpoint、配置版本、请求阶段和稳定错误类别。
- 不记录 Secret、完整请求头、原始响应正文或模型上下文。

## 6. 配置与测试

- 配置由 News Config 加载为不可变快照；加载配置不触发网络请求。
- 未配置的 Provider 不阻断其他 Provider 启动。
- 单元测试使用 Fake HTTPDoer，覆盖请求构造、响应解析、错误映射、分页和部分失败。
- 真实在线探测必须显式启用并使用真实凭证；默认 CI 不依赖外部 Provider 在线可用性。
- Live Probe 结果与离线测试分开报告，不把 skipped 解释为通过。
