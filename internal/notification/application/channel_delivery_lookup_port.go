package application

import (
	"context"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

// ChannelDeliveryLookup 是可选能力；不支持查询的渠道无需实现。
type ChannelDeliveryLookup interface {
	LookupDelivery(ctx context.Context, request domain.ChannelDeliveryLookupRequest) (domain.ChannelDeliveryLookupResult, error)
}
