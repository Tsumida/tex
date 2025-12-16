package pkg

import (
	"context"
	"encoding/json"

	connect "connectrpc.com/connect"
	"github.com/samber/lo"
	"github.com/tsumida/lunaship/log"
	"github.com/tsumida/lunaship/redis"
	"github.com/tsumida/tex/gen/api"
	"github.com/tsumida/tex/gen/api/apiconnect"
	redisstate "github.com/tsumida/tex/pkg/state/redis_state"
	"github.com/tsumida/tex/pkg/texerror"
	"go.uber.org/zap"
)

type TexService struct{}

func NewTexService() *TexService {
	return &TexService{}
}

func (s *TexService) GetOrderList(ctx context.Context, req *connect.Request[api.GetOrderListReq]) (*connect.Response[api.GetOrderListRsp], error) {

	client := redis.GlobalRedis()
	data := redisstate.NewOrderData()
	logger := log.GlobalLog()

	key := data.OrderListKey(req.Msg.AccountId)
	pageOffset := req.Msg.PageOffset
	pageSize := req.Msg.PageLimit
	startIndex := int64(pageOffset * uint32(pageSize))
	endIndex := int64((pageOffset+1)*uint32(pageSize) - 1)
	if pageSize == 0 || pageSize > 100 {
		return connect.NewResponse(&api.GetOrderListRsp{}), texerror.ErrInternal("invalid page size")
	}

	if endIndex > 0 && startIndex >= endIndex {
		logger.Warn("GetOrderList invalid pagination", zap.Any("request", req.Msg))
		return connect.NewResponse(&api.GetOrderListRsp{}), texerror.ErrInternal("invalid pagination")
	}

	orderIDs, err := client.ZRevRange(ctx, key, startIndex, endIndex).Result()
	if err != nil {
		logger.Error("GetOrderList ZRevRange", zap.String("key", key), zap.Int64("start_index", startIndex), zap.Int64("end_index", endIndex), zap.Error(err))
		return nil, err
	}

	// 获取最新的订单ID列表
	rsp := &api.GetOrderListRsp{}
	for _, orderIDBatch := range lo.Chunk(orderIDs, 10) {
		orders, err := client.MGet(ctx,
			lo.Map(orderIDBatch, func(orderID string, _ int) string {
				return data.OrderDetailKey(orderID)
			})...,
		).Result()
		if err != nil {
			logger.Error("GetOrderList MGet", zap.Strings("order_ids", orderIDBatch), zap.Error(err))
			return nil, err
		}

		for _, orderData := range orders {
			if orderData == nil {
				continue
			}
			buf, _ := orderData.(string)
			event := &api.OrderEvent{}
			err = json.Unmarshal([]byte(buf), event)
			if err != nil {
				logger.Error("GetOrderList Unmarshal order data", zap.String("order_data", buf), zap.Error(err))
				continue
			}
			rsp.Orders = append(rsp.Orders, orderDetailFromEvent(event))
		}

	}
	return connect.NewResponse(rsp), nil
}

func (s *TexService) GetOrderDetail(ctx context.Context, req *connect.Request[api.GetOrderDetailReq]) (*connect.Response[api.GetOrderDetailRsp], error) {

	client := redis.GlobalRedis()
	data := redisstate.NewOrderData()
	logger := log.GlobalLog()

	key := data.OrderDetailKey(req.Msg.OrderId)
	orderData, err := client.Get(ctx, key).Result()
	if err != nil {
		logger.Error("GetOrderDetail Get", zap.String("key", key), zap.Error(err))
		return nil, err
	}

	event := &api.OrderEvent{}
	err = json.Unmarshal([]byte(orderData), event)
	if err != nil {
		logger.Error("GetOrderDetail Unmarshal order data", zap.String("order_data", orderData), zap.Error(err))
		return nil, err
	}

	rsp := &api.GetOrderDetailRsp{
		Detail: orderDetailFromEvent(event),
	}
	return connect.NewResponse(rsp), nil
}

func (s *TexService) GetBalance(ctx context.Context, req *connect.Request[api.GetBalanceReq]) (*connect.Response[api.GetBalanceRsp], error) {
	// Scan balance:{account_id}
	client := redis.GlobalRedis()
	data := redisstate.NewBalanceData()
	logger := log.GlobalLog()

	// todo: read from configmap
	keys := data.AllBalance(req.Msg.AccountId, []string{"USDT", "BTC"})
	jsonStrs, err := client.MGet(ctx, keys...).Result()
	if err != nil {
		logger.Error("GetBalance MGet", zap.Strings("keys", keys), zap.Error(err))
		return nil, err
	}

	rsp := &api.GetBalanceRsp{}
	for _, jsonStr := range jsonStrs {
		event := &api.BalanceEvent{}
		buf, _ := jsonStr.(string)
		err = json.Unmarshal([]byte(buf), event)
		if err != nil {
			logger.Error("GetBalance Unmarshal balance data", zap.String("balance_data", buf), zap.Error(err))
			continue
		}
		rsp := &api.GetBalanceRsp{}
		rsp.Balances = append(rsp.Balances, balanceFromBalanceEvent(event))
	}
	return connect.NewResponse(rsp), nil
}

// todo: 实现下单和撤单接口
//  OMS:Bid,1001,CLI_1001_00001,LIMIT,BTC_USDT,10050.00,0.5,GTK,1,false
//  OMS:Ask,1002,CLI_1002_00001,LIMIT,BTC_USDT,9990.00,0.5,GTK,1,false
// pub(crate) fn parse_fill(
//     &self,
//     parts: Vec<&str>,
// ) -> Result<oms::PlaceOrderReq, Box<dyn std::error::Error>> {
//     // 解析指令字段
//     let direction = parts[0];
//     let account_id: u64 = parts[1]
//         .parse()
//         .map_err(|e| format!("AccountID parse error: {}", e))?;
//     let cli_order_id = parts[2];
//     let order_type = parts[3];
//     let trade_pair_str = parts.get(4).ok_or("Missing TradePair")?;
//     let trade_pair = TradePair::from_str(trade_pair_str)?;
//     let price: f64 = parts[5]
//         .parse()
//         .map_err(|e| format!("Price parse error: {}", e))?;
//     let quantity: f64 = parts[6]
//         .parse()
//         .map_err(|e| format!("Quantity parse error: {}", e))?;
//     let time_in_force = parts[7];
//     let stp_strategy: i32 = parts[8]
//         .parse()
//         .map_err(|e| format!("STPStrategy parse error: {}", e))?;
//     let post_only: bool = parts[9]
//         .parse()
//         .map_err(|e| format!("PostOnly parse error: {}", e))?;

//     // 映射枚举值
//     let dir = match direction {
//         "Bid" => oms::Direction::Buy,
//         "Ask" => oms::Direction::Sell,
//         _ => {
//             tracing::error!("Invalid direction: {}", direction);
//             return Err(format!("Invalid direction: {}", direction).into());
//         }
//     } as i32;

//     let ord_type = match order_type {
//         "LIMIT" => oms::OrderType::Limit,
//         "MARKET" => oms::OrderType::Market,
//         _ => {
//             tracing::error!("Invalid order type: {}", order_type);
//             return Err(format!("Invalid order type: {}", order_type).into());
//         }
//     } as i32;

//     let tif = match time_in_force {
//         "GTK" => oms::TimeInForce::Gtk,
//         "IOC" => oms::TimeInForce::Ioc,
//         "FOK" => oms::TimeInForce::Fok,
//         _ => {
//             tracing::error!("Invalid time in force: {}", time_in_force);
//             return Err(format!("Invalid time in force: {}", time_in_force).into());
//         }
//     } as i32;

//     let create_ts_us = chrono::Utc::now().timestamp_micros() as u64;

//     Ok(oms::PlaceOrderReq {
//         order: Some(oms::Order {
//             order_id: "".to_string(), // assigned by OMS
//             client_order_id: cli_order_id.to_string(),
//             direction: dir,
//             account_id,
//             order_type: ord_type,
//             trade_pair: Some(trade_pair.into()),
//             price: format!("{:.2}", price),       // 统一格式化价格
//             quantity: format!("{:.4}", quantity), // 统一格式化数量
//             time_in_force: tif,
//             stp_strategy,
//             post_only,
//             trade_id: 0,
//             prev_trade_id: 0,
//             create_time: create_ts_us,
//             version: 1,
//         }),
//     })
// }

// //  OMS:Cancel,Bid,1001,CLI_1001_00001,BTC_USDT
// //  OMS:Cancel,Ask,1002,CLI_1002_00001,BTC_USDT
// pub(crate) fn parse_cancel(
//     &self,
//     parts: Vec<&str>,
// ) -> Result<oms::CancelOrderReq, Box<dyn std::error::Error>> {
//     let parts: Vec<&str> = parts.into_iter().filter(|p| !p.is_empty()).collect();
//     let direction = parts[0];
//     let account_id: u64 = parts[1]
//         .parse()
//         .map_err(|e| format!("AccountID parse error: {}", e))?;
//     let cli_order_id = parts[2];
//     let trade_pair = TradePair::from_str(parts[3])?;

//     Ok(oms::CancelOrderReq {
//         account_id,
//         order_id: "".to_string(), // assigned by OMS
//         client_order_id: cli_order_id.to_string(),
//         direction: match direction {
//             "Bid" => oms::Direction::Buy as i32,
//             "Ask" => oms::Direction::Sell as i32,
//             _ => {
//                 tracing::error!("Invalid direction: {}", direction);
//                 return Err(format!("Invalid direction: {}", direction).into());
//             }
//         },
//         base: trade_pair.base,
//         quote: trade_pair.quote,
//     })
// }

func (s *TexService) PlaceOrder(ctx context.Context, req *connect.Request[api.PlaceOrderReq]) (*connect.Response[api.PlaceOrderRsp], error) {
	// 转发请求给 oms-server
}

func (s *TexService) CancelOrder(ctx context.Context, req *connect.Request[api.CancelOrderReq]) (*connect.Response[api.CancelOrderRsp], error) {
	// 转发请求给 oms-server
}

var _ apiconnect.TexServiceHandler = (*TexService)(nil)
