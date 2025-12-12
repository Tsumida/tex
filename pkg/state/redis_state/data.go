package redisstate

import "fmt"

// Redis对象读写逻辑

// =========================================== Balance
type Balance struct {
	BalanceKey   func(accountID uint64, currency string) string
	BalanceTsKey func(accountID uint64, currency string) string
}

func NewBalanceData() *Balance {
	return &Balance{
		BalanceKey: func(accountID uint64, currency string) string {
			return fmt.Sprintf("balance:%d:%s", accountID, currency)
		},
		BalanceTsKey: func(accountID uint64, currency string) string {
			return fmt.Sprintf("balance_ts:%d:%s", accountID, currency)
		},
	}
}

// =========================================== Order
type Order struct {
	OrderDetailKey   func(orderID string) string
	OrderDetailTsKey func(orderID string) string
	OrderListKey     func(accountID uint64) string
}

func NewOrderData() *Order {
	return &Order{
		OrderDetailKey: func(orderID string) string {
			return fmt.Sprintf("order_detail:%s", orderID)
		},
		OrderDetailTsKey: func(orderID string) string {
			return fmt.Sprintf("order_detail_ts:%s", orderID)
		},
		OrderListKey: func(accountID uint64) string {
			return fmt.Sprintf("orders:%d", accountID)
		},
	}
}

type OrderLister struct {
	OrderListKey func(accountID uint64) string
}

func NewOrderLister() *OrderLister {
	return &OrderLister{
		OrderListKey: func(accountID uint64) string {
			return fmt.Sprintf("orders:%d", accountID)
		},
	}
}

// =========================================== KBar
var (
	BarSec  = "1s"
	BarMin  = "1m"
	BarHour = "1h"
	BarDay  = "1d"
)

type KBar struct {
	// note: 由于数字币交易所没有开盘收盘，这里Open, Close实际上是一个聚合窗口的开始\最后价。
	Open   string `json:"open"`
	High   string `json:"high"`
	Low    string `json:"low"`
	Close  string `json:"close"`
	Volume string `json:"volume"`

	// 单位秒
	WindowStartInSec  uint64 `json:"start_in_sec"`
	WindowStartInMin  uint64 `json:"start_in_min"`
	WindowStartInHour uint64 `json:"start_in_hour"`
	WindowStartInDay  uint64 `json:"start_in_day"`
}

type KBarData struct {
	TickKey       func(base, quote string) string
	KBarInSecKey  func(base, quote string) string
	KBarInMinKey  func(base, quote string) string
	KBarInHourKey func(base, quote string) string
	KBarInDayKey  func(base, quote string) string
}

func NewKBarData() *KBarData {
	return &KBarData{
		TickKey: func(base, quote string) string {
			return fmt.Sprintf("tick:%s%s", base, quote)
		},
		KBarInSecKey: func(base, quote string) string {
			return fmt.Sprintf("kbar:%s%s:%s", base, quote, BarSec)
		},
		KBarInMinKey: func(base, quote string) string {
			return fmt.Sprintf("kbar:%s%s:%s", base, quote, BarMin)
		},
		KBarInHourKey: func(base, quote string) string {
			return fmt.Sprintf("kbar:%s%s:%s", base, quote, BarHour)
		},
		KBarInDayKey: func(base, quote string) string {
			return fmt.Sprintf("kbar:%s%s:%s", base, quote, BarDay)
		},
	}
}

func (d *KBarData) KeyFns(base, quote string) []string {
	return []string{
		d.TickKey(base, quote),
		d.KBarInSecKey(base, quote),
		d.KBarInMinKey(base, quote),
		d.KBarInHourKey(base, quote),
		d.KBarInDayKey(base, quote),
	}
}
