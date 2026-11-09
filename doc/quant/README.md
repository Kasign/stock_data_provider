# Quant 模块

- 分支：`feat/quant`
- 代码：`internal/quant/**`
- 任务前缀：QNT
- 当前状态：Q0-001～003 盘点/设计工作已完成；QNT-001 CalculationContext 与 Quant Registry 基础能力已提交，Registry 已增加数值精度/JSON 边界校验；QNT-002 八种 Indicator 计算函数、QNT-003 三种时间序列 Factor 函数及固定样本测试已提交。Indicator/Factor 已提交 11 份 synthetic Golden，并新增 Go 测试从文档读取样本、调用算法复算；测试源码尚未获得仓库 CI 通过证据。Registry 数值校验保留 JSON 十进制精度并展开指数记法，避免 float64 往返造成精度位数误判。已增加 CalculationContext 与 Registry 的组合门禁测试源码；合并后 CI 发现 Registry 文件头历史重复声明导致语法失败，已在本轮修复并提交，等待修复后 SHA 的 CI 证据。已增加 Draft 2020-12 Schema 编译/实例校验器，覆盖 Registry Entry、Indicator/Factor 参数 Schema，并禁止外部 Schema 引用；尚缺 Registry 首批注册条目、Golden 全量 Schema 校验、交易日历/复权输入验证和真实 Data Adapter 证据。
- 初始共同基线：`0cb99a525de00d3c340891caa029b716c4d2f749`

Quant 负责确定性、可复算的数值分析：Indicator、Factor、Trend、Market、Industry、Stock、Strategy、Backtest、Risk、Evaluation。不得调用 LLM，不负责研究编排，不直接访问 Provider/HTTP/SQL/Redis。

输入由 Data 提供证券、行情、财务、估值、交易日历、行业历史成分、PIT 和质量契约；输出主要由 Researchflow 消费。Quant 不直接依赖 Research、Researchflow、LLM 或 Notification 的领域实现；风险事件通过本地 Outbox 遵循 INT-CTR-001 发布。

Q0-001～003 仅表示旧资产盘点、字段级核对和 Golden 设计完成，不代表算法实现或真实数据验收完成。QNT-CTR-001～005 的状态以项目级契约目录为准，当前仍未达到 frozen；其中 QNT-CTR-005 已增加 Manifest 构建/Verify、Manifest/AuthorizationRef Schema 与 Golden 校验测试；剩余重点是运行时可信主体/结果归属绑定、读取时当前授权复核、撤销竞态及 CI 证据。Registry 元数据定义已明确，不再因通用 Registry 包边界而阻断实现。

目标结构：

```text
internal/quant/
  domain/
  indicator/
  factor/
  trend/
  market/
  industry/
  stock/
  strategy/
  backtest/
  risk/
  evaluation/
```

首期只按实际任务创建 `domain`、`indicator`、`factor` 等必要包，不为目录齐全创建空包。旧代码迁移结论见 [01-量化引擎总体设计](01-量化引擎总体设计.md) 第 6 节；输入输出契约和 Golden 设计见 [03-首批输入输出契约与验收补充](03-首批输入输出契约与验收补充.md)。

详细任务见 [02-任务拆分与验收](02-任务拆分与验收.md)。


最近一次 Quant 代码验证（2026-11-09）：`e8df200a6843df87a43f2dc4930037e7b12d61b1` 的 Push Workflow `42575135544` 中 `Test go-core` 已通过 gofmt、Go tests、Race Detector 和 go vet。根模块 `Test .` 仍被 Research 文件格式/字节问题阻断，不能标记全仓 CI 通过。


ATR 边界已补充独立 synthetic Golden：明确窗口 N 需要 N 根 OHLC bar，首个 TR 使用输入窗口前一交易日的 previous_close，并以不同 previous_close 的双结果断言证明其参与计算。该样本只锁定算法边界，不代表真实 Data PIT 输入验收。


首批 Factor 已增加 `CalculationContext` 入口，执行前复用共享上下文校验，强制价格计算具备已知复权口径、可信 UTC 截止时间和快照引用；原有纯函数入口保留用于算法复算。该接入不代表交易日连续性验收：Data 交易日历版本的输入字段来源仍待契约明确，Quant 不猜字段、不按自然日推断。


QuantResultRef 已补充构造器输出与 QNT-CTR-004 Draft 2020-12 Schema 的直接一致性测试，覆盖必需授权引用缺失和未知字段拒绝；不将结构校验等同于运行时授权验证。
