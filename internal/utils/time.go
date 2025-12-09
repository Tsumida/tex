package utils

import (
	"fmt"
	"time"
)

var EastEightZone = "Asia/Shanghai"

// GetDayStartTimeMillis 接收一个毫秒时间戳和一个时区字符串，
// 返回该时间戳所在日期在指定时区下的零点（00:00:00）的毫秒时间戳。
func GetDayStartTimeSec(ts int64, zoneName string) (int64, error) {
	loc, err := time.LoadLocation(zoneName)
	if err != nil {
		return 0, fmt.Errorf("无法加载时区 %s: %w", zoneName, err)
	}
	t := time.Unix(ts/1_000_000, (ts%1_000_000)*int64(time.Microsecond)).In(loc)
	dayStartTime := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	return dayStartTime.Unix(), nil
}
