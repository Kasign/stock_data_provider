# LLM 网关配置与真实验收手册

## 1. 目的与边界

本手册说明如何为本服务配置生产 LLM 网关，并由项目负责人执行一次**真实验收**。它覆盖：

- `SERVER_LLM_PROVIDER_JSON` / `SERVER_LLM_CAPABILITY_JSON` / 凭据环境变量的正确填写方式；
- 独立验收命令 `cmd/llm-gateway-check` 的使用方法与通过标准；
- 网关必须满足的协议、TLS、出口策略要求。

它不覆盖、也不得被误读为：

- **完整报告（RunResult）成功路径**。即使本节验收通过，只要 Researchflow 生产 projector 仍为 `nil`、Research Context/Claims/Limitations 与 News `available_at` 未闭环，Run 仍只产生执行摘要，`result_ref` 保持为空。projector 是本项的下游消费者，见 `doc/整体任务规划/06-联调收尾/RepairAll跨模块修复计划与进度.md` 第②项说明。
- 任何凭据明文、网关内网地址或原始响应体的记录。本手册所有示例地址与 ID 均为占位示例，严禁提交真实值。

## 2. 架构约束（不可协商）

1. **协议**：仅 `openai_responses`，对应 `POST {base_url}/responses`。`base_url` 必须是 HTTPS、以 `/v1` 结尾、无 query/fragment、无 userinfo。
2. **TLS**：生产由 OpenResty 终止证书。本服务使用系统根证书校验 HTTPS，**禁止 HTTP、禁止自签证书、禁止重定向**。若 OpenResty 使用私有 CA，须将 CA 装入运行环境信任库，不得关闭校验。
3. **出口绑定**：Server 从 `base_url` 派生固定 DestinationPolicy（单一 host、非 443 端口须显式允许、禁 HTTP、禁重定向、禁 transport 重试）。请求只能发往该绑定目标，运行期不可替换。
4. **能力证据**：Provider 与 Capability 必须成对配置。`responses_json` 必须为 `supported`；只有开启 `stream_enabled` 才要求 `responses_stream` 为 `supported`。证据有有效期，过期即 fail-closed。
5. **凭据**：Provider 只保存 `api_key_secret_ref`（环境变量名），明文由独立环境变量提供；Secret 在所有日志、错误、JSON 输出中被强制脱敏。

## 3. 配置步骤

在 `.env`（或部署环境变量）中填写，模板见仓库根 `.env.example` 的 “LLM Gateway 配置” 段：

1. `SERVER_LLM_PROVIDER_JSON`：单行严格 JSON，字段含义见模板注释。`base_url`、`default_model`、`allowed_models`、`api_key_secret_ref` 必须按真实网关填写。
2. `SERVER_LLM_CAPABILITY_JSON`：单行严格 JSON。`provider_id`/`model_id`/`protocol` 必须与 Provider 一致；`verified_at`/`valid_until` 用真实验收时间重写（RFC3339，Z 结尾，`valid_until - verified_at <= capability_ttl_ms`，且必须覆盖运行时刻）；`evidence_ref` 填可审计证据引用。
3. `<api_key_secret_ref>` 指向的环境变量（示例为 `SERVER_LLM_API_KEY`）：填真实网关调用凭据，不提交。

配置校验发生在 Server 启动阶段（`config.LoadLLM`）：只配一个 JSON、JSON 非法、字段越界、能力与 Provider 不匹配、凭据缺失或证据过期，均导致启动失败，不会静默降级。

## 4. 真实验收

验收命令**不连接 MariaDB / Redis**，只加载 `.env` 与上述配置，通过 Server 真实装配链（`llmtransport.NewResponsesAdapter`）发送一次最小合规请求。

```bash
# 在仓库根目录执行
make llm-gateway-check
# 或
go run ./cmd/llm-gateway-check
```

### 4.1 通过标准

命令输出一行脱敏报告，字段含义：

| 字段 | 含义 |
|---|---|
| `pass` | 是否通过；只有 `true` 才算验收通过 |
| `status` | 响应状态，须为 `completed` |
| `model_requested` | 请求模型 |
| `model_actual` | 网关实际返回模型，**不得为空** |
| `model_version` | 实际模型版本，**不得为空** |
| `response_id` | 网关响应 ID，**不得为空** |
| `gateway_request_id` | 网关请求 ID（可能与上游 ID 分开） |
| `output_chars` | 输出文本字符数 |
| `reason` | 未通过原因（已脱敏） |

通过条件：`pass=true`，等价于 `status=completed` 且 `model_actual`/`model_version`/`response_id` 三项齐全且输出非空。**任一身份字段缺失即 fail-closed**——这与完整报告投影 `ComposeResult` 对 LLM 解释的校验完全一致，因此若网关无法返回这些字段，即使放行也无法进入完整报告。

### 4.2 未通过时

- `llm_gateway_not_configured`：未配置两个 JSON，按第 3 节补齐。
- `gateway_call_failed` 或网关安全消息：检查 TLS 信任、OpenResty 路由、`api_key_secret_ref` 指向的凭据、出口 host/端口白名单。
- `response_status_not_completed` / `llm_identity_incomplete` / `empty_output`：网关返回不符合 `openai_responses` 约定，需在网关侧修正以回传 `model`、`model_version`、响应 `id` 与输出文本。

命令退出码：通过为 0，未通过或配置错误为 1，可用于 CI/脚本门禁。

## 5. 与完整报告成功路径的关系

本节验收只证明“网关可用且响应身份字段可用于非权威解释”。要打通完整报告（`result_ref` 非空），还需同时满足（均不在本手册范围）：

1. Researchflow 生产组合根注入真实 `RunResultProjector`（当前为 `nil`）；
2. Research Context / Claims / Limitations 的生产权威给料；
3. News `available_at` 的真实来源语义与落库闭环（NEWS-BLK-07 / RSC-002）。

上述任一项未闭环时，项目按既有决定不注入占位 projector、不伪造 Provider 响应或能力证据、不生成空壳报告。

## 6. 变更记录

- 2026-11-27：新增本手册与 `cmd/llm-gateway-check`、`.env.example` LLM 配置模板；对应 R4-R5-T1 配置装载与 Adapter 构造能力。
