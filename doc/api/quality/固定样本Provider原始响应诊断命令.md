# 固定样本 Provider 原始响应诊断命令

## 用途与边界

本命令用于对固定样本执行单个 Provider 的真实日 K 请求，保存 HTTP 原始响应并输出该 Provider Adapter 的归一化结果，服务于 Raw → Normalized 字段差分。

固定样本不可由命令参数覆盖：

- 证券：`600519.SH`
- 交易日：`2026-11-09`
- 周期：日 K
- 复权：`none`

每次只允许指定一个 Provider：Eastmoney、Tencent、Sina 或 NetEase。不得循环全市场，不调用 DataService/Application fallback，不连接 MariaDB/Redis，不执行迁移，不写入生产缓存。

## 运行

在仓库根目录、具备正常 DNS/公网访问的部署环境执行：

```bash
go run ./cmd/market-data-diagnostic -provider eastmoney
go run ./cmd/market-data-diagnostic -provider tencent
go run ./cmd/market-data-diagnostic -provider sina
go run ./cmd/market-data-diagnostic -provider netease
```

每次只执行一条命令。输出为 JSON，可重定向到独立文件留存：

```bash
go run ./cmd/market-data-diagnostic -provider eastmoney > eastmoney-600519-20261109.json
```

命令使用 `config.LoadForInProcess` 读取现有 Provider Endpoint 配置，不要求 Data API Key。配置的目标 Endpoint 必须是 HTTPS、标准 443 端口；命令将从实际 Endpoint 提取单一精确 Host，创建只允许该 Host/443 的 Platform BoundDoer。禁止重定向、禁止 Platform 自动重试、禁止 HTTP 及跨域跳转。命令不会放宽白名单来适配未知跳转目标。

## 报告内容

- Provider、固定证券、交易日、周期、复权口径和采集时间。
- 每次实际发生的 HTTP Do 调用：脱敏请求 URL、允许展示的请求头、HTTP 状态、白名单响应头、耗时。
- 原始响应正文以 `raw_payload_base64` 保存，保证 GBK/非法 UTF-8 等原始字节不被 JSON 编码替换；合法 UTF-8 同时提供 `raw_payload_text` 便于阅读，并保留 `body_complete`、`body_bytes`。
- Adapter 输出的 DailyBar，包括 `RawVolume`、`RawVolumeUnit`、`RawAmount`、`RawAmountUnit`、`Volume`、`Amount`、单位、来源、质量状态和采集时间。

敏感 Query 参数（包含 token/key/secret/auth/cookie/password/sign/ut 的参数名）会脱敏；请求/响应头采用白名单输出，不输出 Cookie、Authorization 等头。

## 证据边界

1. 原始正文由 CaptureDoer 在 Provider Adapter 消费前保存，并恢复给 Adapter；捕获器不改变 Adapter 的 Body 读取内容。
2. Platform Client 在返回 Response 前已将正文完整读入内存，并按 `MaxResponseBytes` 限制。超限时 Platform 返回错误，不会提供部分 Response；这类情况报告为请求失败，不可标记为完整捕获。
3. 本次 Eastmoney 日 K 的 `FetchDailyBars` 调用 `getJSON`，不是带重试的 `getJSONWithRetry`；Platform 重试也关闭，因此该固定采集路径不会主动重试。报告仍保留 `requests` 数组，以便未来 Adapter 行为变化时能看见实际调用次数。
4. Provider 私有 Raw 解码结构不对包外暴露。本报告保留原始 HTTP 正文，并通过 DailyBar 暴露 Adapter 实际保留的 Raw 字段；不声称输出了未导出的内部 Raw 结构。
5. 目标 Host 白名单不限制 URL Path；本工具信任当前配置的固定 Provider Endpoint。它不提供 Path allowlist。
6. 只有 `body_complete=true` 的正文可用于完整 Raw 响应差分。DNS 失败、策略拒绝、非 2xx、Adapter 解析失败均须保留原始状态，不能标记为数据通过。

## 当前实现限制

- 只采集日 K，不采集 Quote、复权因子或其他能力。
- 不包含 Tushare/Hithink 凭证 Provider，也不包含 BaoStock TCP。
- 不执行 gofmt/build/test；由 integration 统一验收。
