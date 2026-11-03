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
