# LLM → Platform 公共能力依赖

## 1. 依赖原则

LLM 只消费 Platform 的通用技术原语，不复制网络安全、HTTP Client、凭据装载、Fault 或时间基础设施。Platform 不依赖 LLM，也不拥有模型协议语义。

## 2. 实际使用点

| Platform 能力 | LLM 使用方式 | 所有权边界 |
|---|---|---|
| `platform/config.Secret` | Adapter 构造时显式注入 Gateway Caller Key | SecretRef 解析、环境装载和轮换由 Server/部署负责；LLM 不读环境变量 |
| `platform/httpclient.Client` / `BoundDoer` | LLM 通过窄接口 `HTTPDoer` 发起 JSON 请求 | Client、DNS/IP 校验、DestinationPolicy、超时和传输资源由 Platform 所有 |
| `BoundDoer.OpenStream` / `StreamResponse` | Server 包装为 `llm.HTTPStreamDoer` / `HTTPStreamResponse` | Body 所有权转交 LLM Adapter；Adapter 消费完成后关闭 |
| `platform/fault.Fault` | `ModelErrorFromPlatformFault` 映射到 LLM 稳定错误 DTO | Platform 定义技术 Fault；LLM 定义模型调用错误语义 |
| `platform/time.Clock` | `llm.Clock` 的时间实现别名为 `platformtime.SystemClock`，测试注入确定性 Clock | Platform 提供时钟原语；LLM 不自建全局时间服务 |

## 3. 不得重复建设

- 不得在 LLM 创建 `http.Client`、Transport、DNS/IP 校验、重定向控制或 SSRF 防护。
- 不得在 LLM 解析进程环境变量或自行读取 SecretRef。
- 不得创建第二套通用 Fault、Diagnostics Runner 或时间服务。
- 不得把 Platform 的 A1 HTTP Client 测试当作真实 Gateway 能力验收。

## 4. 错误映射规则

LLM 将 Platform Fault 投影为 ModelErrorInfo，不能泄露 Cause、Dependency 或底层响应正文。流式传输超限 Fault 的代码为 `platform.httpclient.stream_body_too_large`，必须与非流式 `platform.httpclient.response_too_large` 一样映射为 `resource_limit`；该映射由 LLM 负责并有回归测试。

## 5. 验收边界

- PASS：Platform Fault/BoundDoer 接口符合契约，LLM 映射不泄露内部字段，Body 所有权正确。
- DEGRADED：可识别的可选传输能力未配置，但调用入口明确拒绝相应操作。
- FAIL：SSRF/目标策略被绕过、Body 泄漏、Fault 暴露敏感 Cause，或资源超限被错误归类。
- NOT_COMPARABLE：只有 Mock/Fixture，尚无真实 Gateway/部署证据。

真实 DNS/egress/Secret 部署验收归 Server/Platform；LLM 只验收其消费边界。
