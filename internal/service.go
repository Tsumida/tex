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

func (s *TexService) GetOrderList(context.Context, *connect.Request[api.GetOrderListReq]) (*connect.Response[api.GetOrderListRsp], error) {
	panic("todo")
}
func (s *TexService) GetOrderDetail(context.Context, *connect.Request[api.GetOrderDetailReq]) (*connect.Response[api.GetOrderDetailRsp], error) {
	panic("todo")
}
func (s *TexService) GetBalance(context.Context, *connect.Request[api.GetBalanceReq]) (*connect.Response[api.GetBalanceRsp], error) {
	panic("todo")
}

var _ apiconnect.TexServiceHandler = (*TexService)(nil)
