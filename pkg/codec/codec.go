package codec

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tsumida/tex/gen/api"
)

// Cmd2PlanceReq 将命令行字符串转换为 api.PlaceOrderReq 请求。
// 示例命令格式: OMS:Bid,1001,CLI_1001_00001,LIMIT,BTC_USDT,80000.00,1.0,GTK,1,false
func Cmd2PlaceReq(cmd string) (*api.PlaceOrderReq, error) {
	// 1. 预处理和分割
	// 移除 OMS: 前缀，确保分割后的第一个元素是 Bid/Ask
	cleanCmd := cmd
	if strings.HasPrefix(cleanCmd, "OMS:") {
		cleanCmd = cleanCmd[len("OMS:"):]
	} else {
		return nil, fmt.Errorf("命令格式错误: 缺少 'OMS:' 前缀")
	}

	parts := strings.Split(cleanCmd, ",")

	// 检查字段数量是否足够（至少 10 个字段）
	if len(parts) < 10 {
		return nil, fmt.Errorf("命令格式错误: 字段数量不足 (预期 >= 10), 得到 %d 个字段", len(parts))
	}

	// 2. 字段解析与类型转换（参照 Rust 逻辑）

	// 0. Direction (Bid/Ask)
	directionStr := parts[0]
	var direction int32
	switch strings.ToUpper(directionStr) {
	case "BID":
		direction = int32(api.Direction_Buy)
	case "ASK":
		direction = int32(api.Direction_Sell)
	default:
		return nil, fmt.Errorf("Invalid direction: %s", directionStr)
	}

	// 1. AccountId (u64)
	accountId, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("AccountID parse error: %v", err)
	}

	// 2. ClientOrderId (string)
	cliOrderId := parts[2]

	// 3. OrderType (LIMIT/MARKET)
	orderTypeStr := strings.ToUpper(parts[3])
	var orderType int32
	switch orderTypeStr {
	case "LIMIT":
		orderType = int32(api.OrderType_Limit)
	case "MARKET":
		orderType = int32(api.OrderType_Market)
	default:
		return nil, fmt.Errorf("Invalid order type: %s", orderTypeStr)
	}

	// 4. TradePair (string -> TradePair)
	pairParts := strings.Split(parts[4], "_")
	if len(pairParts) != 2 {
		return nil, fmt.Errorf("TradePair 格式错误 (预期 BASE_QUOTE): %s", parts[4])
	}
	tradePair := &api.TradePair{
		Base:  pairParts[0],
		Quote: pairParts[1],
	}

	// 5. Price (string) - 避免浮点数精度问题，直接使用字符串
	priceStr := parts[5]
	// (如果需要验证价格格式，可以尝试解析为 f64 但最终存为 string)
	// priceF64, err := strconv.ParseFloat(parts[5], 64)
	// if err != nil { ... }
	// priceStr := fmt.Sprintf("%.2f", priceF64) // 如果需要重新格式化

	// 6. Quantity (string) - 避免浮点数精度问题，直接使用字符串
	quantityStr := parts[6]
	// (如果需要验证数量格式，可以尝试解析为 f64 但最终存为 string)
	// quantityF64, err := strconv.ParseFloat(parts[6], 64)
	// if err != nil { ... }
	// quantityStr := fmt.Sprintf("%.4f", quantityF64) // 如果需要重新格式化

	// 7. TimeInForce (GTK/IOC/FOK)
	tifStr := strings.ToUpper(parts[7])
	var timeInForce int32
	switch tifStr {
	case "GTK":
		timeInForce = int32(api.TimeInForce_GTK)
	case "IOC":
		timeInForce = int32(api.TimeInForce_IOC)
	case "FOK":
		timeInForce = int32(api.TimeInForce_FOK)
	default:
		return nil, fmt.Errorf("Invalid time in force: %s", tifStr)
	}

	// 8. StpStrategy (i32)
	stpStrategyInt, err := strconv.ParseInt(parts[8], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("STPStrategy parse error: %v", err)
	}
	stpStrategy := int32(stpStrategyInt)

	// 9. PostOnly (bool)
	postOnly, err := strconv.ParseBool(parts[9])
	if err != nil {
		return nil, fmt.Errorf("PostOnly parse error: %v", err)
	}

	// 10. CreateTime (u64, microseconds)
	// 参照 Rust 使用 Utc::now().timestamp_micros()
	createTime := uint64(time.Now().UnixMicro())

	// 3. 构造 Order 和 PlaceOrderReq
	order := &api.Order{
		ClientOrderId: cliOrderId,
		Direction:     direction,
		AccountId:     accountId,
		OrderType:     orderType,
		TradePair:     tradePair,
		Price:         priceStr,
		Quantity:      quantityStr,
		TimeInForce:   timeInForce,
		StpStrategy:   stpStrategy,
		PostOnly:      postOnly,
		CreateTime:    createTime,
		// OrderId, TradeId, PrevTradeId 等字段留空或使用默认值，由 OMS 填充
	}

	return &api.PlaceOrderReq{
		Order: order,
	}, nil
}

// OMS:Cancel,Bid,1001,CLI_1001_00101,BTC_USDT
func Cmd2CancelReq(cmd string) (*api.CancelOrderReq, error) {
	cleanCmd := cmd
	if strings.HasPrefix(cleanCmd, "OMS:") {
		cleanCmd = cleanCmd[len("OMS:"):]
	} else {
		return nil, fmt.Errorf("命令格式错误: 缺少 'OMS:' 前缀")
	}

	parts := strings.Split(cleanCmd, ",")

	if len(parts) < 4 {
		return nil, fmt.Errorf("命令格式错误: 字段数量不足 (预期 >= 4), 得到 %d 个字段", len(parts))
	}

	// 0. Direction (Bid/Ask)
	directionStr := parts[1]
	var direction int32
	switch strings.ToUpper(directionStr) {
	case "BID":
		direction = int32(api.Direction_Buy)
	case "ASK":
		direction = int32(api.Direction_Sell)
	default:
		return nil, fmt.Errorf("Invalid direction: %s", directionStr)
	}

	// 1. AccountId (u64
	accountId, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("AccountID parse error: %v", err)
	}

	// 2. ClientOrderId (string)
	cliOrderId := parts[3]

	// 3. TradePair (string -> TradePair)
	pairParts := strings.Split(parts[4], "_")
	if len(pairParts) != 2 {
		return nil, fmt.Errorf("TradePair 格式错误 (预期 BASE_QUOTE): %s", parts[4])
	}
	base := pairParts[0]
	quote := pairParts[1]

	return &api.CancelOrderReq{
		AccountId:     accountId,
		ClientOrderId: cliOrderId,
		Direction:     direction,
		Base:          base,
		Quote:         quote,
	}, nil
}
