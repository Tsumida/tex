package tex

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/tsumida/tex/gen/api"
	"github.com/tsumida/tex/gen/api/apiconnect"

	"github.com/tsumida/tex/pkg"
)

func TestMain(m *testing.M) {
	go pkg.RunApp(context.TODO())
	time.Sleep(5 * time.Second)
	m.Run()
}

func TestGetOrderList_OK(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	for _, order := range rsp.Msg.Orders {
		assert.Equal(t, api.OrderState_name[int32(api.OrderState_Cancelled)], order.CurrentState)
	}

	req.AccountId = 1002
	rsp, err = client.GetOrderList(ctx, &connect.Request[api.GetOrderListReq]{Msg: req})
	assert.NoError(t, err)
	assert.NotNil(t, rsp)
	assert.Greater(t, len(rsp.Msg.Orders), 0)
	for _, order := range rsp.Msg.Orders {
		assert.Equal(t, api.OrderState_name[int32(api.OrderState_Filled)], order.CurrentState)
	}
}
