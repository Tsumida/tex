package tex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/tsumida/tex/gen/api"
	"github.com/tsumida/tex/gen/api/apiconnect"

	"github.com/tsumida/tex/pkg"
)

func prettyPrint(i any) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	return string(s)
}

func TestMain(m *testing.M) {
	go pkg.RunApp(context.TODO())
	time.Sleep(2 * time.Second)
	m.Run()
}

// 期望：正常返回订单列表
func TestGetOrderList_OK(t *testing.T) {
	ctx := context.Background()
	req := &api.GetOrderListReq{
		AccountId:  1001,
		PageOffset: 0,
		PageLimit:  10,
	}

	_, handler := apiconnect.NewTexServiceHandler(pkg.NewTexService())
	ts := httptest.NewServer(handler)
	defer ts.Close()

	client := apiconnect.NewTexServiceClient(ts.Client(), ts.URL)

	rsp, err := client.GetOrderList(ctx, &connect.Request[api.GetOrderListReq]{Msg: req})
	assert.NoError(t, err)
	assert.NotNil(t, rsp)
	assert.Greater(t, len(rsp.Msg.Orders), 0)

	req.AccountId = 1002
	rsp, err = client.GetOrderList(ctx, &connect.Request[api.GetOrderListReq]{Msg: req})
	assert.NoError(t, err)
	assert.NotNil(t, rsp)
	assert.Greater(t, len(rsp.Msg.Orders), 0)
}

// 期望：分页正常，超出范围后无数据返回
func TestGetOrderList_Pagination(t *testing.T) {
	ctx := context.Background()

	req := &api.GetOrderListReq{
		AccountId:  1001,
		PageOffset: 0,
		PageLimit:  2,
	}

	_, handler := apiconnect.NewTexServiceHandler(pkg.NewTexService())
	ts := httptest.NewServer(handler)
	defer ts.Close()

	client := apiconnect.NewTexServiceClient(ts.Client(), ts.URL)

	// 第1，2页，有数据
	rsp, err := client.GetOrderList(ctx, &connect.Request[api.GetOrderListReq]{Msg: req})
	assert.NoError(t, err)
	assert.NotNil(t, rsp)
	assert.True(t, len(rsp.Msg.Orders) > 0)
	req.PageOffset += 1
	rsp, err = client.GetOrderList(ctx, &connect.Request[api.GetOrderListReq]{Msg: req})
	assert.NoError(t, err)
	assert.NotNil(t, rsp)
	assert.True(t, len(rsp.Msg.Orders) > 0)

	// 第3页，无数据
	req.PageOffset += 100
	rsp, err = client.GetOrderList(ctx, &connect.Request[api.GetOrderListReq]{Msg: req})
	assert.NoError(t, err)
	assert.NotNil(t, rsp)
	assert.Equal(t, 0, len(rsp.Msg.Orders))
}

func TestGetOrderDetail(t *testing.T) {
	ctx := context.Background()
	req := &api.GetOrderListReq{
		AccountId:  1001,
		PageOffset: 0,
		PageLimit:  10,
	}

	_, handler := apiconnect.NewTexServiceHandler(pkg.NewTexService())
	ts := httptest.NewServer(handler)
	defer ts.Close()

	client := apiconnect.NewTexServiceClient(ts.Client(), ts.URL)

	rsp, err := client.GetOrderList(ctx, &connect.Request[api.GetOrderListReq]{Msg: req})
	assert.NoError(t, err)
	assert.NotNil(t, rsp)
	assert.Greater(t, len(rsp.Msg.Orders), 0)

	orderID := rsp.Msg.Orders[0].Original.OrderId

	detailReq := &api.GetOrderDetailReq{
		AccountId: 1001,
		OrderId:   orderID,
	}

	detailRsp, err := client.GetOrderDetail(ctx, &connect.Request[api.GetOrderDetailReq]{Msg: detailReq})
	assert.NoError(t, err)
	assert.NotNil(t, detailRsp)

	order := detailRsp.Msg.Detail.Original
	assert.Equal(t, orderID, order.OrderId)
	fmt.Println(prettyPrint(order))
}
