package model

// ProviderProbeOptions 描述一次 Provider 能力探测请求。
// 字段来源均为调用方显式请求；空切片/空字符串按 DataService 现有默认规则解释。
type ProviderProbeOptions struct {
	Scope               string            // 探测范围；合法值由 Provider 探测服务校验。
	Capabilities        []string          // 指定能力名称；为空时使用服务端默认能力集合。
	Checks              []string          // 指定检查项；为空时使用服务端默认检查集合。
	Markets             []string          // 限定市场；为空时不额外限制市场。
	Symbols             []string          // 指定样本证券代码；为空时使用服务端样本。
	DailyBarAdjustments []AdjustmentBasis // 日线探测使用的复权口径。
	DailyBarStartDate   string            // 日线探测窗口起始交易日，包含边界，YYYY-MM-DD。
	DailyBarEndDate     string            // 日线探测窗口结束交易日，包含边界，YYYY-MM-DD。
	TimeoutSeconds      int               // 单次探测超时秒数；有效范围由服务端校验和限制。
}
