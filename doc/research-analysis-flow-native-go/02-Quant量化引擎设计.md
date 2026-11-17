# Quant 量化引擎设计

## 1. 设计目标

Quant 是确定性、可复算、可测试的数值分析引擎。相同输入快照、参数、算法版本和交易日历必须产生可解释且可重复的结果。Quant 不调用 LLM，不直接访问 Provider、数据库、Redis 或 HTTP。

不把当前简单 MA20/5 日收益/20 日收益实现视为目标引擎。可以复用经过验证的数学实现，但必须重新核对公式、复权、缺失值、边界和 PIT 行为。

## 2. 模块边界

| 模块 | 职责 | 主要输出 |
|---|---|---|
| indicator | 基础技术指标和统计函数 | IndicatorValue |
| factor | 标准化因子、横截面处理、因子版本 | FactorValue |
| trend | 趋势方向与阶段、周期一致性 | TrendState |
| market | 指数、全市场宽度、量能、波动、风险偏好 | MarketRegime |
| industry | 行业强度、广度、贡献、持续性、轮动 | IndustryStrength |
| stock | 个股多周期状态、量价、波动、相对强度 | StockQuantSnapshot |
| strategy | 策略定义、信号生成、退出和约束 | SignalCandidate |
| backtest | 历史撮合模拟、成本、交易制度和统计 | BacktestResult |
| risk | 硬风险规则、风险预算、拒绝原因 | RiskDecision |
| evaluation | 样本外、稳定性、漂移和概率校准 | EvaluationReport |

这些是逻辑模块，不要求每个目录立即独立部署或独立数据库。

## 3. 计算输入契约

QuantInput 至少包含：
- security_id / exchange / instrument_type；
- as_of、交易日历版本、复权模式和 adjustment_reference_date；
- 输入快照引用和每个数据域的 source/version/quality；
- OHLCV、成交额、交易状态、指数/行业成员关系（按任务需要）；
- 财务/估值字段及 report_date、available_at、system_observed_at；
- 计算窗口、参数和算法版本。

所有数值字段必须明确单位、精度、空值语义和适用范围。未知值使用显式状态，不得用零代替缺失。历史研究必须使用当时可用的证券主数据、行业成分、财务和复权因子版本。

## 4. 统一结果契约

MetricValue：
- metric_id、value、unit、period、as_of；
- source_refs、input_snapshot_ref；
- quality、coverage、reason_codes；
- algorithm_id、algorithm_version、parameters_hash。

TrendState：
- direction：bullish / neutral / bearish / unknown；
- phase：strong / advancing / pullback / consolidating / rebound / weakening / reversal_watch / unknown；
- horizon：short / medium / long；
- confidence 只能表达规则/模型证据强弱，不能覆盖 direction/phase；
- evidence_metric_refs、quality、reason_codes。

SignalCandidate：
- strategy_id/version、security_id、as_of；
- side、entry_condition、invalidation_condition、exit_condition；
- expected_holding_period、supporting_metric_refs；
- sample_stats_ref、risk_constraints_ref；
- status：candidate / rejected / blocked / eligible_for_review；
- 不等同交易指令，不连接券商下单。

RiskDecision：
- decision：pass / warn / block / unknown；
- hard_rules[]、violations[]、risk_budget_ref；
- block 不允许被分数或 LLM 改写。

## 5. 分析维度

### 5.1 趋势

趋势方向和趋势阶段是两个独立维度。按短/中/长周期计算：
- 价格结构：高低点、均线斜率/排列、突破与回撤；
- 动量：多周期收益、动量变化和相对强度；
- 趋势质量：持续性、波动调整后强度、量价配合；
- 反转观察：结构破坏、动量衰减、异常波动；
- 周期冲突：短周期与长周期相反时显式输出冲突，不强行合并。

MA 是特征之一，不是趋势结论的唯一依据。阈值、窗口和参数必须版本化，且通过历史样本验证。

### 5.2 市场环境

必须明确市场统计母体、交易状态、覆盖率和时间点：
- 指数趋势和主要宽基指数相对表现；
- 全市场上涨/下跌/平盘家数、涨跌幅分布；
- 创新高/新低、均线之上比例等市场宽度；
- 成交额、成交活跃度、量能变化；
- 波动水平、极端波动、涨跌停家数；
- 流动性和风险偏好代理指标；
- 市场阶段及证据，不输出脱离样本和规则的主观标签。

自选股/当前分页统计不得称为全市场统计。退市、停牌、新股、ST 和不同板块规则必须定义分母口径。

### 5.3 行业/主题

- 分类体系和成员关系必须带版本及历史有效区间；
- 行业收益、相对强度、横截面广度、成分贡献分开计算；
- 行业上涨不能自动解释为资金净流入；
- 主题关联与正式行业分类分开；
- 轮动需有连续时间序列、排名变化和持续性，不以单日涨幅代替趋势。

### 5.4 个股

- 多周期趋势和相对强度；
- 量价关系、成交活跃度、波动与流动性；
- 缺口、突破、回撤和关键价格结构；
- 财务、估值和筹码只使用可证明在 as_of 时点可用的数据；
- 停牌、ST、上市时间、退市风险和交易限制进入质量/风险门禁；
- 输出支持和反向指标，不能只返回单一综合分。

## 6. 策略与回测

策略接口必须分离信号生成、交易模拟和绩效统计。回测必须处理：
- A 股 T+1、涨跌停不可成交、停牌、ST、上市/退市和交易日历；
- 复权与公司行动，禁止前视；
- 手续费、印花税、过户费（适用时）、滑点和成交约束；
- 开盘/收盘/盘中成交假设明确，不能使用未来价格成交；
- 样本内、验证集、样本外和滚动窗口；
- 基准、超额收益、胜率、盈亏比、回撤、换手、容量和交易次数；
- 失败案例、参数敏感性和策略失效监控。

没有足够交易样本或数据质量不满足要求时，结果标记 insufficient_sample/degraded，不输出“策略有效”。

## 7. 风险门禁

硬规则与软风险评分分离。硬规则至少覆盖：
- 证券交易状态、停牌、ST、退市/上市状态；
- 数据缺失、过期、PIT 不完整和质量无效；
- 流动性、涨跌停和成交可行性；
- 策略适用范围和最大风险预算；
- 回测配置、样本数量和风险约束不合法。

硬规则 block 必须优先于任何综合分、情景概率和 LLM 解释。未知状态不得默认通过。

## 8. 评估与防过拟合

- 固定研究样本和数据快照；
- 时间切分和 walk-forward；
- 多重检验、参数搜索偏差和幸存者偏差检查；
- 预测误差、概率校准、分层稳定性和市场状态漂移；
- 训练/评估数据严格按时间隔离；
- 每个模型/策略版本可追溯，支持版本间比较和回滚。

## 9. 旧代码处置

现有 `internal/analysis/**`、`go-core/internal/indicators/**`、`go-core/internal/market/**` 等作为候选资产清单，而非目标结构。每个候选实现按以下标准处置：
1. 算法定义和单位是否正确；
2. PIT、复权、缺失和异常边界是否正确；
3. 是否有独立测试和固定样本；
4. 是否被其他模块依赖；
5. 是否能迁移到新契约且不引入旧架构耦合。

结论只能是 reuse、rewrite、retire、defer，并记录理由。未完成评估前不批量删除；完成替换并确认调用方迁移后再清理。
