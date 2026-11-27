package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
)

// 全量涨跌停池取数的失败分类。
//
// 区分这两类不是措辞问题：源不可用（网络/协议/超时）重试可能成功，
// 源返回不自洽的池（部分行、下界总量）重试没有意义。上层诊断与告警需要据此分流。
var (
	ErrLimitPoolSourceUnavailable = errors.New("limit-pool source unavailable")
	ErrLimitPoolSourceIncomplete  = errors.New("limit-pool source returned an incomplete or inconsistent pool")
)

// FetchFullLimitPool 按配置的 Provider 顺序读取指定交易日的**全量**涨跌停池。
//
// 该方法是「可交易性事实」的唯一来源：只有能表达「某证券不在池内」的完整池
// 才能支撑 price_limit 结论。因此：
//   - 只接受实现 provider.FullLimitPoolProvider 的来源，限额接口一律跳过；
//   - 返回前复核 Total == len(Items) 且非下界，任一不成立即视为该来源失败；
//   - 绝不以部分池冒充完整池，也不跨来源拼接。
func (s *DataService) FetchFullLimitPool(
	ctx context.Context, poolType, tradeDate string,
) (*model.LimitPoolSnapshot, error) {
	if s == nil || len(s.limitPoolProviders) == 0 {
		return nil, fmt.Errorf("%w: no enabled limit-pool provider", ErrLimitPoolSourceUnavailable)
	}
	poolType = strings.ToLower(strings.TrimSpace(poolType))
	if poolType != "up" && poolType != "down" {
		return nil, fmt.Errorf("unsupported limit-pool type %q; want up or down", poolType)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var failures []string
	incomplete := false
	for _, candidate := range s.limitPoolProviders {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		full, ok := candidate.(provider.FullLimitPoolProvider)
		if !ok {
			failures = append(failures, candidate.Name()+": provider cannot return a full limit pool")
			continue
		}
		snapshot, fetchErr := full.FetchFullLimitPool(ctx, poolType, tradeDate)
		if fetchErr != nil {
			failures = append(failures, candidate.Name()+": "+fetchErr.Error())
			continue
		}
		if err := validateFullLimitPool(poolType, tradeDate, candidate.Name(), snapshot); err != nil {
			incomplete = true
			failures = append(failures, candidate.Name()+": "+err.Error())
			continue
		}
		// 零事件池是合法结果：是否可用由载荷层结合同日兄弟池判定，这里原样返回。
		return snapshot, nil
	}
	sentinel := ErrLimitPoolSourceUnavailable
	if incomplete {
		sentinel = ErrLimitPoolSourceIncomplete
	}
	if len(failures) == 0 {
		return nil, fmt.Errorf("%w: all limit-pool providers returned no usable data", sentinel)
	}
	return nil, fmt.Errorf("%w: %s", sentinel, strings.Join(failures, "; "))
}

func validateFullLimitPool(poolType, tradeDate, providerName string, snapshot *model.LimitPoolSnapshot) error {
	if snapshot == nil {
		return errors.New("limit-pool provider returned an empty snapshot")
	}
	if snapshot.PoolType != poolType {
		return fmt.Errorf("limit-pool type mismatch: requested %s got %q", poolType, snapshot.PoolType)
	}
	if snapshot.TradeDate != tradeDate {
		return fmt.Errorf("limit-pool trade_date mismatch: requested %s got %q", tradeDate, snapshot.TradeDate)
	}
	if strings.TrimSpace(snapshot.Source) == "" || snapshot.Source != providerName {
		return errors.New("limit-pool source is missing or does not match the provider")
	}
	// 零行是合法形态：当日该池没有涨跌停证券（强势日跌停常为 0）。
	// 「零事件」还是「该交易日不可用」是日期级歧义，必须由载荷层结合同日兄弟池判定；
	// 取数层把它升级成硬错误，会让 price_limit 在没有跌停的日子整体不可用。
	if len(snapshot.Items) == 0 {
		if snapshot.Total != 0 {
			return fmt.Errorf("limit-pool is partial: total=%d items=0", snapshot.Total)
		}
		return nil
	}
	if !snapshot.HasData {
		return errors.New("limit-pool provider returned no data flag")
	}
	if snapshot.TotalIsLowerBound {
		return errors.New("limit-pool total is a lower bound; completeness cannot be asserted")
	}
	if snapshot.Total != len(snapshot.Items) {
		return fmt.Errorf("limit-pool is partial: total=%d items=%d", snapshot.Total, len(snapshot.Items))
	}
	return nil
}
