package markettime

import (
	"testing"
	"time"
)

func TestLocationUsesAsiaShanghai(t *testing.T) {
	first := Location()
	second := Location()
	if first == nil || first != second {
		t.Fatal("A 股业务时区应返回同一个已缓存的时区对象")
	}
	if first.String() != "Asia/Shanghai" {
		t.Fatalf("时区名称 = %q，期望 Asia/Shanghai", first.String())
	}
	instant := time.Date(2026, 11, 17, 16, 0, 0, 0, time.UTC)
	if got := instant.In(first).Format("2006-01-02 15:04:05"); got != "2026-11-18 00:00:00" {
		t.Fatalf("UTC 时间转换结果 = %s，期望 2026-11-18 00:00:00", got)
	}
}
