# Quant 模块

- 分支：`feat/quant`
- 代码：`internal/quant/**`
- 任务前缀：QNT
- 状态：规划中，尚未开始模块分支开发

Quant 负责确定性、可复算的数值分析：Indicator、Factor、Trend、Market、Industry、Stock、Strategy、Backtest、Risk、Evaluation。不得调用 LLM，不负责研究编排，不直接访问 Provider/HTTP/SQL/Redis。

输入由 Data 提供证券、行情、财务、估值、交易日历、行业历史成分、PIT 和质量契约；输出由 Researchflow 消费。禁止依赖 Research、Researchflow、LLM、Notification。

目标结构：

```text
internal/quant/
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

设计文档：[01-量化引擎总体设计](01-量化引擎总体设计.md)。

详细任务见 [02-任务拆分与验收](02-任务拆分与验收.md)。
