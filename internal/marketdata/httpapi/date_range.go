package httpapi

import "time"

// inclusiveCalendarDays 返回包含起止日期的自然日数量。调用方应先验证 end 不早于 start。
func inclusiveCalendarDays(start, end time.Time) int {
	return int(end.Sub(start)/(24*time.Hour)) + 1
}
