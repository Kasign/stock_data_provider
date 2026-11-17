# GOV-CTR-001 公共 JSON 类型、数值与空值规则

- 契约编号：GOV-CTR-001
- 规则版本：1.0.0
- 状态：freeze-ready（待项目负责人确认冻结）
- 适用范围：所有跨模块 JSON DTO、事件、持久化公开引用和 API 请求/响应
- 权威性：本文件是 GOV-CTR-001 公共语义的唯一权威来源。各领域 DTO/Schema/Registry 负责字段级适用定义，不得复制或重新解释本规则。

## 1. JSON 类型选择

| 类型 | 适用语义 | 约束 |
|---|---|---|
| string | 标识、代码、枚举、精确 Decimal 数值文本 | 非空规则、格式和长度由字段 Schema 声明；空字符串默认无效 |
| integer | 离散计数/数量 | 仅表达整数；JSON number 绝对值不得超过 9,007,199,254,740,991 |
| number | 允许浮点误差的统计数值 | 必须声明单位、范围、精度、容差和输出舍入策略 |
| boolean | 明确的二态事实 | 不允许用 null、0、空字符串替代 |
| array/object | 有序集合或结构化 DTO | 必须由 Schema 定义元素/属性；不得用任意 map 替代强类型对象 |

超出 JavaScript 安全整数范围的整数不得通过 JSON number 传输；生产方必须使用有明确语义的字符串编码或拒绝。消费者不得先转成 number 再计算。

## 2. Decimal string

要求精确十进制表达的金额、价格、单价、成本、费用等字段必须使用普通十进制字符串，不得使用 JSON number。

规范语法：

`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$`

禁止科学计数法、前导加号、前导多余零、千分位、货币符号、空白、空字符串及 NaN/Infinity 等非有限值。

每个字段/Metric 的生产方必须声明：
- 是否允许负值；
- 最大整数位数；
- 最大 fractional digits / scale；
- 是否保留尾随零；
- 最小值/最大值；
- 如确需舍入，明确舍入模式和容差。

未声明舍入模式时禁止自动舍入。不得先经 float64 再格式化为 Decimal string。无法证明源精度或无法无损转换时，必须明确失败，或按对应领域契约输出 degraded/unknown；不得声称精确。

## 3. Integer 与 Number

- integer 只表示离散计数/数量。字段生产方声明最小值、最大值及是否允许负值。
- number 只用于业务明确允许浮点误差的统计量；字段生产方必须声明 unit、range、precision/tolerance 和输出舍入策略。
- 不存在项目级统一舍入模式。各字段不得自行隐式舍入；未声明舍入策略时禁止自动舍入。
- NaN、Infinity、-Infinity 不得进入 JSON。
- lower_bound、upper_bound 必须与 value 使用相同 JSON 类型、单位和精度规则；区间必须同时为空或同时存在，且满足 lower_bound <= value <= upper_bound（适用条件由字段 Schema 明确）。

## 4. 单位与比例

- 金额、价格、股数、手数、成交量、成交额、换手率、收益率、概率等必须声明明确单位/计量口径；消费者不得根据字段名或 Provider 习惯推断。
- 比例必须明确使用 `ratio` 或 `percentage_point`，不得只写“百分比”。
- `0.0125 ratio` 与 `1.25 percentage_point` 数值不同；换算只能在显式 Adapter 边界按明确公式执行。
- 每个 Metric/Indicator/Factor 的单位、精度、舍入、容差和算法版本由其生产模块 Registry 管理，消费者不得自行解释。
- 本规则不规定所有金额/价格统一小数位，不要求所有数值字段一律使用 Decimal string，也不替代 Provider 原始精度保留。

## 5. 缺失、空值和质量状态

| 表达 | 规范语义 | 禁止解释 |
|---|---|---|
| 字段缺失 | optional 字段按 DTO 条件/版本不适用或未提供 | 不得表达“已评估但未知”，不得自动补零 |
| null | 字段存在但当前无可提供值；具体含义由字段定义 | 不得等同字段缺失 |
| 0 | 有证据的有效数值零 | 不得当作缺失 |
| empty string | 默认无效，除非字段明确允许 | 不得当作 null/unknown |
| empty array | 已确认的空集合 | 不得等同缺失、未知或未执行 |
| unknown | 已评估但无法判断 | 不得当作 false、0 或 success |
| unavailable | 依赖或能力当前不可用 | 不得当作无数据或空集合 |
| not_supported | 当前不支持该能力/类型 | 不得伪装成空结果 |
| not_configured | 必需配置尚未提供 | 不得标记为成功 |
| missing | 按契约应存在的数据缺失 | 不得自动降级为正常 |
| degraded | 有结果但存在已知质量/覆盖限制 | 不得隐去限制 |
| invalid | 输入或输出不满足契约 | 不得继续作为有效事实传播 |

required+nullable 字段必须输出字段键，并在无值时显式输出 null；不得使用 omitempty 省略。optional 字段是否可 null 必须由字段 Schema 明确。字符串字段不得使用 `"unknown"` 作为数值缺失标记；未知状态使用领域 status/reason_code 表达。

质量状态与执行状态分离。HTTP 成功、请求成功或任务 completed 不代表数据/结果 valid。质量状态由事实/结果生产方负责产生，消费者不得自行升级。

## 6. 字段级责任边界

本规则只定义跨模块共用的 JSON/数值/空值原则。每个公开字段仍由生产方在 DTO/JSON Schema/OpenAPI/Registry 中声明：
- JSON 类型、required/optional、nullable、条件必需性；
- unit/currency/share/lot/ratio 等计量单位；
- 最大整数位、scale、尾随零策略、range、负值许可；
- number 的 precision、tolerance、rounding；
- default、缺失/null/空数组/unknown 的字段特定含义。

字段级缺口由对应领域 CTR 跟踪，例如 DATA-CTR-001/002/004/005、QNT-CTR-001～004、RSC-CTR-001/002、NEWS-CTR-001、LLM-CTR-001 等；不得反向阻断 GOV-CTR-001 公共规则本身冻结。

## 7. Golden 最小覆盖要求

每个字段生产方的 Golden/Contract Test 至少按适用情况区分：
- 有效正数、有效零值、允许的负值；
- 精度/scale 边界、舍入边界和超范围值；
- 字段缺失、显式 null、空字符串；
- 空数组与非空数组；
- unknown/unavailable/degraded/invalid；
- JavaScript 安全整数边界及超界；
- ratio 与 percentage_point 的不同数值表达。

不适用的情形由字段 Schema 明确排除，不要求所有字段机械覆盖所有类别。
