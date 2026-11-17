package markettime

import (
	"fmt"
	"sync"
	"time"

	platformtime "example.com/stock-analysis-flow/internal/platform/time"
)

var (
	locationOnce sync.Once
	location     *time.Location
	locationErr  error
)

// LoadLocation 返回缓存的 A 股业务时区；时区数据通过 Platform 时间服务统一加载。
func LoadLocation() (*time.Location, error) {
	locationOnce.Do(func() {
		location, locationErr = platformtime.LoadLocation("Asia/Shanghai")
	})
	return location, locationErr
}

// Location 返回 A 股业务时区；固定时区数据不可用时明确失败，不静默回退到固定 UTC+8。
func Location() *time.Location {
	loc, err := LoadLocation()
	if err != nil {
		panic(fmt.Errorf("加载 A 股业务时区 Asia/Shanghai 失败: %w", err))
	}
	return loc
}
