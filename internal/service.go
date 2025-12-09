package internal

import (
	"context"

	connect "connectrpc.com/connect"
	"github.com/tsumida/tex/gen/api"
	"github.com/tsumida/tex/gen/api/apiconnect"
)

type TexService struct{}

func NewTexService() *TexService {
	return &TexService{}
}

func (s *TexService) GetOrderList(ctx context.Context, req *connect.Request[api.GetOrderListReq]) (*connect.Response[api.GetOrderListRsp], error) {
	// client := infra.GlobalRedis()
	// data := redis_state.NewOrderData()

	// 从Redis的List获取
	// data.OrderListKey(req.Msg.AccountId)

	panic("todo")
}

func (s *TexService) GetOrderDetail(ctx context.Context, req *connect.Request[api.GetOrderDetailReq]) (*connect.Response[api.GetOrderDetailRsp], error) {
	panic("todo")
}

func (s *TexService) GetBalance(ctx context.Context, req *connect.Request[api.GetBalanceReq]) (*connect.Response[api.GetBalanceRsp], error) {
	panic("todo")
}

var _ apiconnect.TexServiceHandler = (*TexService)(nil)
