# Go 与 Python 数据服务实现及数据质量对比报告

- 检查分支：`feat/full-go-data-providers-api-switch`
- 检查基线：`d2f5740be3f8f6365788b1c59ed43fa0f1d5f197`
- 报告性质：代码/文档静态对照 + Go 质量契约自动化回归；不等同于对所有上游的实时数据抓取对账。
- 原则：已实现不代表生产可用；CI 通过不代表字段级等价；未核实的字段必须保持 unknown/null/degraded，不能猜测补齐。

## 1. 总结

1. A 股主行情路径已经有原生 Go 服务，包含多个独立 Provider、质量归一化、缓存/存储和显式质量状态。
2. Python 代码没有全部删除。仓库仍有 `python_bridge/`、`internal/adapters/pythonapi/`、`go-core/internal/adapters/pythonbridge/`、`cmd/pythonapi-smoke/`。Python bridge 仍服务新闻搜索、LLM、通知、持久化以及部分兼容/辅助能力；旧 Python 行情 API 适配器也仍在仓库中。
3. 原 Python `DataFetcherManager` / Efinance / AkShare 是多源聚合和按能力回退，不是一个单一 Provider。Go 已覆盖大量国内能力，但不能据此声称 Efinance/AkShare 每个函数和字段都 1:1 迁移。
4. 最明显的数据质量差异：旧 Python bridge 对日 K 明确把来源、单位、复权基准标为 unknown，并把缺失字段记录在 `missing_fields`；Go 原生模型记录来源、单位、复权基准、质量状态/原因、抓取时间、规范化时间及部分 Provider 时间，且执行更严格的输入校验。
5. 当前迁移主要缺口是字段级对账、历史时点可重放性、部分数据域覆盖、来源真实网络稳定性和目标部署环境验收。

## 2. 服务与代码实现清点

| 层级 | 当前实现 | 判断 |
|---|---|---|
| 原生行情服务 | `services/market-data`：HTTP API、Provider 注册与路由、归一化、缓存、MariaDB repository、数据质量状态 | A 股行情数据层主实现 |
| Go 分析核心 | `go-core`：分析流程、领域模型、Go market-data HTTP client、新闻搜索适配器 | 核心流程可使用 Go 行情服务；新闻链仍保留 bridge fallback |
| 旧 API 适配器 | `internal/adapters/pythonapi`、`cmd/pythonapi-smoke` | 代码保留；需要确认是否仍有部署/调用方依赖后再删除 |
| Go Core 兼容桥 | `go-core/internal/adapters/pythonbridge` | 仍存在兼容调用，不能称为 Python 全面下线 |
| Python bridge | `python_bridge/capabilities/{market_data,search,llm,persistence,notification}.py` 等 | Python 运行时和部分能力仍在 |
| 原 Python 数据源管理器 | 由上游 `daily_stock_analysis` 的 `DataFetcherManager` 提供；当前仓库没有完整复制原始 Provider 实现 | 本仓库无法单靠本地文件完成所有旧 Provider 函数级运行对比，需以原上游版本和固定样本补齐基准 |

### 国内 Provider 覆盖（按能力概括）

| 能力域 | Go 实现/来源 | 主要限制或待验收 |
|---|---|---|
| 实时行情 | 东方财富、腾讯、新浪；Pytdx、TickFlow、可选 Yahoo 等能力按开关注册 | 时间戳、字段覆盖、单位和上游稳定性需按来源分别验收 |
| 日 K | 东方财富、腾讯、新浪、BaoStock；另有 Pytdx、TickFlow、Tushare、网易及可选 Yahoo 等能力 | 复权算法不完全等价；缺成交额不得估算；不同 adjustment 不可跨源拼接 |
| 证券搜索/基础资料 | 腾讯 Smartbox、东方财富目录/F10；可选 THS/Tushare | 分页完整性、字段覆盖及批量资料口径仍需对照 |
| 指数 | 核心指数快照、指数目录/成分/日 K 的独立 Provider | 当前成分快照不等于历史纳入/剔除生效区间；指数与个股代码空间不同 |
| 市场广度/板块 | 涨跌分布、全市场成交额、行业/概念排行 | 多页数据可能不是原子快照；时间戳或全量覆盖证据不足时应 degraded |
| 涨跌停池/龙虎榜 | 东方财富及可选 THS/Tushare | 分页总数、报告日期、金额单位、权限及完整覆盖仍需验收 |
| 财务/估值 | BaoStock、可选 Tushare/THS；六组标准指标与三张报表模型 | 历史修订版本重建、严格 PIT、字段等价和账户权限不完整 |
| 资金流/筹码 | 东方财富资金流与 CYQ 计算，可选妙想补充 | 精确 available_at/发布时间缺失；CYQ 是估算快照，不应伪装为历史事实 |
| 新闻/公告 | Go 原生公告及多个新闻候选源；部分链路仍保留 Python bridge | 新闻桥未完全移除；覆盖率、来源时间与真实生产搜索仍需验收 |
| 海外市场 | 部分 Yahoo 可选代码；Longbridge/Futu/Finnhub/Alpha Vantage 未迁移 | 按既定范围暂缓，不作为当前国内 A 股迁移阻塞项 |

## 3. Python 与 Go 数据质量契约对比

| 质量维度 | 旧 Python 兼容层现状 | Go 原生行情服务现状 | 对比结论 |
|---|---|---|---|
| 来源标识 | 行情桥能传部分 source；旧日 K 来源来自 manager 返回值 | 每条行情有 source/source_version，支持 fallback 信息 | Go 元数据更完整，但来源是否真实、路由是否等价仍需实测 |
| 交易代码 | 旧桥以字符串传入，市场类型由旧 manager 处理 | 统一六位代码与 SH/SZ/BJ 后缀校验，并拒绝 B 股误入 A 股契约 | Go 的边界校验更明确；需对照旧系统代码别名和指数代码 |
| OHLC | Python 将缺失/非法数值转为 0，随后丢弃 OHLC 不完整行 | Go 校验正数、有限数、OHLC 顺序、日期范围和重复日期；坏行剔除并留下原因 | Go 的诊断和显式契约更强；需关注“丢行后是否降级”是否贯穿 API |
| 缺失成交量/成交额 | Python bar helper 将缺失字段记录在 missing_fields，但数值字段仍可能以 0 表示；行情桥把旧日 K 的单位标 unknown | Go 使用指针表达 null；成交额缺失保留 null、amount_unit=unknown，并将质量降级 | Go 避免把未知金额伪装成 0/CNY，属于重要质量改进 |
| 成交量单位 | 旧桥无法可靠确认各 Fetcher 的手/股差异，标 unknown | Provider 按来源契约转换成 shares；腾讯等存在市场/端点差异的专门分支 | Go 更严格；必须用真实 payload 验证每个来源，不能只凭测试夹具 |
| 成交额单位 | 旧桥不能可靠确认币种/单位 | 有金额时要求标准 CNY；缺失时单位 unknown；对不复权 K 线做金额/成交量数量级检查 | Go 质量门禁更严格，但金额单位仍需供应商证据 |
| 复权基准 | 旧日 K 标记 adjustment_basis=unknown，不能确认原 Fetcher 实际返回的复权口径 | Go 明确 none/forward_adjusted/backward_adjusted/unknown；未知口径不能进入规范序列；不同算法可记录 adjustment_method | Go 可解释性更好；算法名称相同也不保证跨供应商数值完全相同 |
| 涨跌幅 | Python helper 解析缺失时记录 missing_fields，数值可能为 0 | Go 校验非有限值；部分 Provider 可推导涨跌幅，但需记录来源/降级原因 | 需统一定义“上游字段”与“安全推导字段”的来源语义 |
| 时间语义 | 旧桥的 fetched_at 是桥接抓取时间，不等于供应商发布时间；历史 as_of 时跳过实时/辅助快照 | Go 区分 fetched_at、normalized_at、quote_time/provider_timestamp；部分域仍缺精确 available_at | Go 模型更完善，但不少 Provider 没有可靠源端时间，不能标 valid 了事 |
| PIT/历史重放 | Python 旧辅助接口不支持完整历史快照，历史请求会跳过当前筹码/财务/板块 | Go 对部分 PIT 请求明确拒绝或降级；部分财务版本/指数成分历史仍不完整 | Go 的失败边界更清晰，但完整 PIT 尚未实现 |
| 空结果/错误 | Python bridge 以 warnings 和空数据返回为主，具体 manager 失败语义分散 | Go 服务有质量状态、错误原因、Provider fallback、unknown/degraded/stale 等状态 | Go 可观测性更强；需验证每个服务层都保留 Provider 质量原因 |
| 排名/池子覆盖 | 旧 manager 由源端和 Fetcher 分页/返回规则决定 | Go 部分接口显式检查分页、总数、报告日和截断 | Go 已改进，但全市场覆盖审计仍未全域完成 |
| 历史时点财务/公司行动 | 无法可靠重建原时点的公告版本与复权因子 | Tushare 按公告日期选择可用版本；历史修订版本和公司行动因子仍有缺口 | 不能把当前财报值用于历史 PIT 回测 |

## 4. 质量测试覆盖和本次新增用例

新增 `services/market-data/internal/normalize/quality_parity_test.go`，覆盖下列契约：

1. 对旧桥那种单位/复权基准 unknown 的日 K，Go 不猜测、不偷偷归一化，而是拒绝进入规范序列。
2. 成交额缺失时保留 null，金额单位改为 unknown，质量标记 degraded 并记录原因。
3. 非法 OHLC 与重复交易日必须剔除，且产生可机器断言的诊断代码。
4. 非有限行情字段和不复权 K 线明显不合理的金额/成交量数量级必须拒绝。

仓库原有测试还覆盖了涨跌幅一致性、非有限数字、价格/金额尺度异常、单位转换、日期范围、limit 截断、复权价格不与原始成交额错误交叉校验等情况。

### 本次自动化测试能证明什么

- Go 归一化函数按当前契约处理指定的合成样本。
- 已明确记录旧桥 unknown 元数据与 Go 强契约之间的行为差异。
- 可以在 GitHub Actions 重复执行并防止回归。

### 本次自动化测试不能证明什么

- 不能证明真实 Python manager 和 Go Provider 对每个证券/日期返回完全相同的价格和字段。
- 不能证明上游单位/权限/限流文档正确，也不能证明所有 Provider 的实时网络可用。
- 不能代替 NAS/Docker、MariaDB/Redis 和完整 API 路由验收。
- 由于当前仓库没有完整复制旧 Python Provider 的实现，真正的逐函数差分测试还需要固定原上游版本、建立脱敏样本夹具，并在相同证券、日期、复权基准下分别采集结果。

## 5. 主要差异与风险分级

### P0：不可接受的错误数据风险

- 缺失成交额被写成 0 或标成 CNY。
- 未确认手/股时错误换算成交量。
- 未复权数据被当作前复权回退，或不同来源/不同复权算法的 K 线被拼接。
- 历史分析混入当前实时行情、当前财报/筹码或当前指数成分。
- 分页不完整却返回“完整市场”且没有 degraded/truncated 说明。

Go 当前已为上述多项建立门禁，但需要确认这些门禁从 Provider 到服务 API、存储与分析上下文全链路保留。

### P1：迁移等价缺口

- Efinance/AkShare 聚合行为、优先级、异常回退语义没有逐函数对账。
- 行业/概念、人气榜、涨跌停池、龙虎榜和财务字段未完成统一固定样本比较。
- 指数成分历史生效区间、历史财报修订与公司行动因子不足。
- Python bridge 辅助能力未完全迁移。

### P2：生产验收缺口

- 目标部署网络与上游稳定性。
- Provider 权限、限流与响应 schema 变化。
- 缓存 freshness、持久化迁移、健康检查与端到端任务链路。

## 6. 建议的后续差分验收方案

每个数据域使用同一组证券、交易日、请求时间和复权基准建立 Python/Go 双侧样本。至少包含沪深主板、创业板、科创板、北交所、停牌/无数据日、非交易日、复权事件、缺失字段和上游错误。

每条样本对比：
- 字段存在率、null/zero 差异、数值单位和精度；
- OHLC、成交量、成交额、涨跌幅；
- 交易日覆盖、重复/乱序/超范围行；
- adjustment_basis、adjustment_method、报告日、公告时间、available_at；
- source、quality_status、quality_reasons、fallback 来源；
- API 状态码、超时/限流/空结果行为。

比较前先标准化代码、时区、单位和复权算法。不同复权算法的绝对价格不应直接判等；应比较同一算法下的序列，或明确记录算法差异。所有不能证实的字段都应列为 unknown/degraded，而不是用容差掩盖。

## 7. 验收结论

**当前判断：Go 原生行情服务的质量契约总体比旧 Python 兼容桥更明确，尤其是单位、复权、null、来源和质量状态；但“实现覆盖”与“字段级等价”是两件事。**

在完成逐数据域固定样本差分测试之前，不应宣布 Python 到 Go 的全量等价迁移已经完成。Python 代码清理应安排在：Go 覆盖已确认、双侧对账通过、实际调用方迁移、生产回归通过之后。

参考：
- [Python Provider → Go 迁移矩阵](./PYTHON_PROVIDER_GO_MIGRATION_MATRIX.md)
- [Go 数据服务迁移状态](./GO_DATA_PROVIDER_SWITCH_STATUS.md)
- [数据源扩展计划](./DATA_SOURCE_EXPANSION_PLAN.md)
- [真实环境验收记录](./DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md)
