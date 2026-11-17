package transport

import (
	"context"
	"net"
)

// DialContextFunc 是 Root 注入的受控 TCP 出站连接函数。
// network/address 必须按配置目标进行精确校验；调用方传入的 context 控制连接超时和取消。
type DialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)
