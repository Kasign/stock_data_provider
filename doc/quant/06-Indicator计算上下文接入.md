# Indicator 计算上下文接入

## 目标

为首批八种 Indicator 提供统一的上下文入口，使纯计算函数在执行前复用 Quant 的 `CalculationContext`，避免调用方绕过 system_as_of、输入快照、复权口径和质量策略的结构门禁。

## 实现

- 新增 `internal/quant/indicator/context.go`，为 SMA、EMA、Return、Volatility、RSI、MACD、ATR、Bollinger 提供 `WithContext` 入口。
- 所有入口先调用 `CalculationContext.Validate(trustedNow, true)`，再委托给既有纯计算函数；公式、窗口和结果结构保持不变。
- `trustedNow` 必须由 Application/可信 Clock 注入，Indicator Domain 不读取系统时钟。
- ATR 的 `previousClose` 必须与窗口 OHLC 使用同一复权口径，且由调用方提供。

## 边界

该接入只保证计算上下文结构和价格复权口径字段已声明且可比较，不证明 snapshot_ref 真实存在、行情 PIT 正确、交易日连续、停牌/缺失分类正确或输入数据来自可信 Data Adapter。上述数据事实统一留待 Data Adapter 与后续真实数据验收。

## 后续

Application 应优先调用 `WithContext` 入口，不直接调用无上下文纯函数。纯函数保留给算法组合、Golden 和不涉及外部数据边界的内部计算使用。
