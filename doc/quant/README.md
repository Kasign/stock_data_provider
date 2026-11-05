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
  indicator/ factor/ trend/ market/ industry/
  stock/ strategy/ backtest/ risk/ evaluation/
```

关键规则：MA20 只是特征；市场统计明确全集/分母/覆盖率；行业成员历史版本化；成交额不等于净流入；回测包含 T+1、涨跌停、停牌、ST、上市退市、费用滑点和公司行动；硬风险 block 不可被评分覆盖。

详细任务见 [02-任务拆分与验收](02-任务拆分与验收.md)。
