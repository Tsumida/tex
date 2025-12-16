package pkg

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	connect "connectrpc.com/connect"
	"github.com/samber/lo"
	"github.com/tsumida/lunaship/log"
	"github.com/tsumida/lunaship/redis"
	"github.com/tsumida/tex/gen/api"
	"github.com/tsumida/tex/gen/api/apiconnect"
	redisstate "github.com/tsumida/tex/pkg/state/redis_state"
	"github.com/tsumida/tex/pkg/texerror"
	"go.uber.org/zap"
	"golang.org/x/net/http2"

	iutils "github.com/tsumida/tex/pkg/utils"
)

type TexService struct {
}

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

func (s *TexService) PlaceOrder(ctx context.Context, req *connect.Request[api.PlaceOrderReq]) (*connect.Response[api.PlaceOrderRsp], error) {
	// 转发请求给 oms-server
	client := NewOmsClient("http://oms-server:8080")
	return client.PlaceOrder(ctx, req)
}

func (s *TexService) CancelOrder(ctx context.Context, req *connect.Request[api.CancelOrderReq]) (*connect.Response[api.CancelOrderRsp], error) {
	// 转发请求给 oms-server
	client := NewOmsClient("http://oms-server:8080")
	return client.CancelOrder(ctx, req)
}

var _ apiconnect.TexServiceHandler = (*TexService)(nil)

var globalOmsClient apiconnect.OMSServiceClient = nil
var initOnce sync.Once

func NewOmsClient(omsServerUrl string) apiconnect.OMSServiceClient {
	initOnce.Do(func() {
		httpClient := iutils.NewH2CClient(1*time.Second, func(t *http2.Transport) {
			t.PingTimeout = 1 * time.Second
		})
		globalOmsClient = apiconnect.NewOMSServiceClient(httpClient, omsServerUrl,
			connect.WithGRPC(),
		)
	})
	return globalOmsClient
}
