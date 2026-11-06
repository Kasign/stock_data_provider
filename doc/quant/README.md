# Quant 模块

- 分支：`feat/quant`
- 代码：`internal/quant/**`
- 任务前缀：QNT
- 当前状态：Q0-001 旧资产盘点、Q0-002 契约核对、Q0-003 Schema/Golden 方案设计已完成；QNT-001~003 尚未开始实现
- 最近盘点基线：`0cb99a525de00d3c340891caa029b716c4d2f749`

Quant 负责确定性、可复算的数值分析：Indicator、Factor、Trend、Market、Industry、Stock、Strategy、Backtest、Risk、Evaluation。不得调用 LLM，不负责研究编排，不直接访问 Provider/HTTP/SQL/Redis。

输入由 Data 提供证券、行情、财务、估值、交易日历、行业历史成分、PIT 和质量契约；输出由 Researchflow 消费。禁止依赖 Research、Researchflow、LLM、Notification。

当前仅完成旧代码资产盘点和迁移边界识别，不代表 Quant 算法、Schema、Golden、Data Adapter 或真实数据验收完成。QNT-CTR-001～005 当前均为 `pending_evidence`，按具体调用点逐项解除阻断。

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

首期只按实际任务创建 `domain`、`indicator`、`factor` 等必要包，不为目录齐全创建空包。旧代码迁移结论见 [01-量化引擎总体设计](01-量化引擎总体设计.md) 第 6 节和 [02-任务拆分与验收](02-任务拆分与验收.md) 第 6 节。

详细任务见 [02-任务拆分与验收](02-任务拆分与验收.md)。
