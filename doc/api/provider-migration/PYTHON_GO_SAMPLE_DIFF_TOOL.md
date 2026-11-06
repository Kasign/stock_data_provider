# Python / Go 样本差分工具

工具：`scripts/compare_market_data_exports.py`

## 目的与边界

将固定 Python Provider 基线与 Go market-data 服务导出的同一批样本按字段比较。工具只读取本地 JSON，不发起网络请求、不调用 Provider、不生成模拟行情。没有真实 Python 导出时，不得将 Go 黄金样例复制成 Python 输入。

## 统一导出格式（schema_version=1）

Python 与 Go 两侧各导出一个 JSON 文件，顶层结构如下：

```json
{
  "schema_version": 1,
  "dataset": "daily_bars",
  "symbol": "600519.SH",
  "adjustment_basis": "none",
  "provider": "python-efinance",
  "captured_at_utc": "2026-10-30T07:00:00Z",
  "source_commit": "固定源码提交或 Go 服务提交 SHA",
  "provider_version": "Provider/SDK 版本",
  "endpoint": "实际命中的接口标识",
  "request_params": {"adjust": "qfq", "period": "daily"},
  "raw_payload_sha256": "64 位原始响应 SHA-256",
  "records": [
    {
      "trade_date": "2026-10-29",
      "open": 1500.0,
      "high": 1510.0,
      "low": 1490.0,
      "close": 1505.0,
      "volume": 1000000,
      "volume_unit": "shares",
      "amount": null,
      "amount_unit": "unknown",
      "quality_status": "degraded",
      "quality_reasons": ["amount_missing"]
    }
  ]
}
```

以上仅为字段结构示例，不是实际行情样本。每一侧必须保留自身真实输出、Provider、采集时间和代码版本。实时行情可将 `dataset` 设为 `quote`，并将 `--key` 指定为 `symbol` 或双方共同的稳定观察键。

## 执行

```bash
python3 scripts/compare_market_data_exports.py \
  --python /path/to/python-export.json \
  --go /path/to/go-export.json \
  --key trade_date \
  --output /path/to/parity-report.md
```

可按字段覆盖默认容差：

```bash
python3 scripts/compare_market_data_exports.py \
  --python python.json --go go.json \
  --tolerance close=0.001 --tolerance volume=0
```

退出码：`0` 表示没有阻断性字段差异；`1` 表示记录覆盖或字段存在阻断差异；`2` 表示输入结构/样本身份错误。质量状态与质量原因差异单独列入报告，并同样使退出码为 1；质量语义不一致不能被当作数值容差通过。

## 真实样本凭证

真实准确性验收必须增加 `--require-real-provenance`。该模式要求 Python 和 Go 两侧分别提供 source_commit、provider、provider_version、endpoint、request_params、captured_at_utc（显式 UTC）和 raw_payload_sha256。provider 不能是 fixture/mock/synthetic/unknown；source_commit 必须是完整 Git SHA；raw_payload_sha256 必须是 64 位 SHA-256。该模式只验证凭证字段结构和摘要格式，不证明原始响应本身真实；原始响应文件必须另行归档并按摘要复核。

不带该选项的模式继续用于合成 fixture 的 CI 合约测试，不能作为真实数据准确性验收。

## 比较规则

- 必须匹配 `dataset`、`symbol`、`adjustment_basis`；不匹配直接拒绝比较。
- 默认以 `trade_date` 对齐记录，可用 `--key` 指定其他键；重复键直接报错。
- 数值默认容差：价格/涨跌额 0.01、涨跌幅 0.001、成交量 0、成交额 0.01；应根据字段单位和来源精度显式复核。
- null 与 0 不相等；缺失字段与显式 null 不相等。
- 来源、版本、Provider 时间、采集时间等来源元数据不参与两侧相等判断，但保留在报告头部。
- 质量状态/原因差异单独标记；Go 更严格的质量判定应作为兼容差异保留，不得为了通过差分而弱化 Go 质量门禁。
- 工具不自动转换单位、不自动转换复权、不补齐缺失字段、不忽略额外记录。

## 当前状态

工具和导出协议已具备；旧 Python 固定提交为 `Kasign/daily_stock_analysis@7a21cd76952c7fe44a7fc5a6113a5b2b8a16bb32`。真实双侧差分仍需在可运行该 Python 依赖的环境导出同证券、同交易日、同复权基准的数据后执行。当前 Go 双 Provider 黄金样例不能替代 Python 样本。


## CI 合约测试与真实准确性验收的边界

当前 data-quality-parity.yml 执行的是比较器单元测试、Go 归一化契约测试和静态标记检查。test_compare_market_data_exports.py 中的 Python/Go 数据均由 export() 在测试内构造，Provider 标记为 fixture；例如 close=10.005 对 close=10.0 是容差边界测试，不是 Python 真实行情。测试中出现的 2026-10-28/2026-10-29 差异同样是人为构造的覆盖率/字段差异用例。

因此：
- 该 CI 只能证明比较器和质量规则按预期工作，不能证明旧 Python 行情准确，也不能证明 Go 与 Python 真实数据等价。
- 真实数据验收必须使用固定 Python 源码运行后导出的原始 Provider 样本；禁止复制 Go 样本伪装 Python 样本。
- 每份真实导出必须标明 source_commit、provider、provider_version、request_params、captured_at_utc，并附带未经清洗的原始 payload 或其可审计摘要。
- 报告应分开给出“比较器合约 CI”与“真实 Provider 数据验收”状态；前者通过不能自动提升后者状态。
