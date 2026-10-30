# Go 与 Python 数据服务实现及数据质量对比报告

- 检查分支：`feat/full-go-data-providers-api-switch`
- 历史质量契约检查基线：`d2f5740be3f8f6365788b1c59ed43fa0f1d5f197`
- 本轮静态盘点基线：`4c603172448102058f95e6bc625494436a8402c9`（2026-10-30）
- 当前 Provider 代码能力、默认开关和 Python bridge 调用以 [迁移矩阵](./PYTHON_PROVIDER_GO_MIGRATION_MATRIX.md) 为准；本报告不代表已完成真实请求对账。
- 报告性质：代码/文档静态对照 + Go 质量契约自动化回归；不等同于对所有上游的实时数据抓取对账。
- 原则：已实现不代表生产可用；CI 通过不代表字段级等价；未核实的字段必须保持 unknown/null/degraded，不能猜测补齐。

## 0. 必须保留的 Go 数据质量与单位统一改进（迁移保护声明）

**这是明确的维护约束：Go 原生数据服务已经实现的单位统一、数据质量校验和质量元数据能力必须保留，不得为了追求与旧 Python 返回值表面一致、简化代码或清理旧实现而删除、弱化或绕过。** Python/Go 对比的目标是识别能力差异并补齐迁移，不是把 Go 降级到旧 Python 的质量水平。

具体约束：

1. **单位规范化必须保留**：按 Provider/端点真实契约，将成交量统一到标准 shares、成交额统一到标准 CNY；保留各来源必要的专门转换分支。没有供应商依据或无法确认单位时，必须标记 unknown/degraded 并阻止不可信数据进入规范序列，不得猜测换算，也不得把未知单位直接当作标准单位。
2. **缺失值语义必须保留**：成交额、成交量及其他可缺失字段应继续区分 null、真实数值 0 和未知值；不得将缺失值填成 0，不得为满足旧接口兼容而伪造 CNY 或 shares。
3. **复权语义必须保留**：明确区分 none、forward_adjusted、backward_adjusted、unknown，并保留 adjustment_method 等必要信息；未知复权基准或算法不明确的数据不得静默混入规范序列，不得跨不同复权口径/算法拼接。
4. **质量门禁与诊断必须保留**：非有限数值、非法 OHLC、重复/越界日期、数量级异常、分页/覆盖不完整等现有校验，以及 quality_status、quality_reasons、source、fallback 和时间元数据，均不得在 Provider、API、缓存、存储或分析层传递过程中被丢弃或降级成无法解释的结果。
5. **兼容优先适配边界**：若旧 Python 调用方需要旧字段名、结构或展示格式，应在 API/适配器边界做显式兼容转换，并保留 Go 内部标准模型与质量状态；不得反向修改 Go 内部标准单位和质量契约来迎合旧行为。
6. **修改前后必须有回归保护**：任何涉及单位换算、金额/成交量校验、复权、null/zero、Provider fallback 或质量状态传播的改动，都应补充或更新测试；测试不能只验证成功路径，也应覆盖未知单位、缺失字段、异常尺度和错误复权口径。
7. **清理旧代码的前置条件**：只有在调用方迁移、真实样本差分验证和部署回归均通过后，才可删除不再使用的 Python 兼容代码；不得把清理 Python 代码与删除 Go 质量改进绑定为同一项重构。

后续任何迁移矩阵、差分测试和重构评审都应遵循本声明。发现旧 Python 行为与 Go 质量契约冲突时，默认保留 Go 的严格处理，并记录为兼容差异；只有经过明确的数据契约评审，才能调整规则。

## 1. 总结

本轮阶段一已补充当前分支 Provider 代码/配置静态盘点。矩阵区分“代码实现”“默认开关”和“真实运行验收”，不能将其中任一项替代另外两项。

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

## 3.1 国内日 K / 行情单位实现核对（代码级，不等同真实源端验收）

下表是对当前 Go Provider 代码与现有单测的静态核对结果。它用于定位回归保护缺口；没有供应商原始文档或真实 payload 佐证的内容，仍须保留相应质量限制。

| Provider / 端点 | 成交量输入与标准输出 | 成交额处理 | 已有保护 / 仍需注意 |
|---|---|---|---|
| Tushare 日 K | 日 K 原始量按手转换为 shares，并保留 `RawVolumeUnit=hands` | 原始 amount 按 thousand_CNY 转换为 CNY；保留原始值/单位；缺失时为 null + unknown + degraded | 已增加单位回归测试；还需真实响应样本确认接口字段契约未变化 |
| 东方财富实时行情 / 日 K | 实时行情与日 K 的原始量按 hands 转为 shares | 实时行情字段 f48、日 K 字段 kline field 6 按 CNY 记录 | 代码有转换路径；需继续确保所有异常数值和源端字段变化都会降级或拒绝 |
| 腾讯实时行情 | 使用端点字段与成交额/换手率等指标校验 shares/hands；无法确认时拒绝猜测 | 按已识别字段提取；金额未知时不伪造标准值 | 有显式单位确认逻辑；K 线没有已验证成交额字段，必须继续保持 null/unknown/degraded |
| 腾讯日 K | 常规 A 股 K 线按 hands 转换；科创板代码 688/689 按 shares 处理 | 目前没有已验证的成交额字段，不估算金额 | 专门分支容易被“统一简化”误删，必须保留并由真实样本持续验证 |
| BaoStock 日 K | 按 shares 读取，不再乘以 100；输出标准 shares | 有值时按 CNY 记录；缺失时 null + unknown + degraded | 已测试金额、单位、复权元数据、缺失金额和异常 OHLC；供应商协议/字段语义仍应独立核验 |
| TickFlow 实时行情 / 日 K | 按 shares 读取，输出标准 shares | 有效 amount 按 CNY；缺失/非法 amount 保留 unknown 并降级 | 已覆盖 shares 单位和缺失/可选数组等契约；API schema 变化需通过真实环境验收 |
| Pytdx 日 K | 对来源量执行 shares 归一化；不把未知尺度直接视作可信值 | 金额与成交量不一致时将金额置 null、单位 unknown 并降级；非有限/负值拒绝 | 已有金额/成交量一致性、非法数字测试；不同市场/服务器返回语义仍需固定样本 |
| 新浪日 K | 按 shares 读取 | K 线金额未可靠确认时保持 unknown，并降级 | 避免伪造金额是有意质量约束；不要为填满字段而估算成交额 |
| 网易日 K / 实时行情 | 根据量价与金额尺度判断 shares 或 hands；推断分支必须留下单位标记与 degraded 原因 | 有效金额按 CNY；不能验证单位时保留 unknown/degraded | 推断不是供应商显式单位声明；真实样本需单独核验，不能把 inferred 当作 fully verified |

### 代码审计结论

- **已确认的实现保护点**：标准成交量以 shares 表达；已确认金额以 CNY 表达；原始单位/字段应保留；缺失金额不得用 0 代替；腾讯、网易等来源的单位差异不能被简单合并。
- **需继续核验的边界**：上游接口文档与真实 payload 的字段含义、不同市场的特殊量纲、金额字段是否在复权 K 线下仍具同一口径，以及 Provider 质量原因是否穿过 API/缓存/存储层。
- **证据等级**：本表属于当前代码和测试的静态核对，不代表已经完成所有供应商的在线数据对账；真实样本差分仍是单独的验收项。

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


## 8. 第四阶段 PIT 实现增量（2026-10-30）

- 日 K PIT 前/后复权现改为从已记录的未复权日 K 与同一 `as_of` 下可证明可用的复权因子重建；输出记录复权参考日期、因子 Provider、因子版本及系统观察时间。
- 严格复权公式遵循 Tushare 累计后复权因子定义：前复权为 `raw × factor / reference_factor`，后复权为 `raw × factor`。本次实现只建立可复现的 Go 计算路径，不表示当前 Tushare 因子具备 PIT 可用时间证据。
- Tushare `adj_factor` 未暴露精确首次可用时间；Tushare `dividend` 仅给日期字段；Tushare `index_weight` 是月度权重报告，未给出成员有效起止日期。相关严格 PIT 查询必须排除/降级，不能伪造时间字段。
- 历史指数成分成员模型已分离 `effective_from/effective_to` 与权重报告日期，并在区间不覆盖查询日期、缺失时间证据或时间顺序异常时降级。
- 因子和公司行动可见性门禁要求 `fetched_at`、`available_at`、`system_observed_at` 均不晚于 `as_of`；已补充相应边界测试。
- 本节为代码静态状态。Go 测试、Python/Go 固定样本差分、真实 Provider 权限和 MariaDB 部署验收尚未执行，不应标记为通过。
