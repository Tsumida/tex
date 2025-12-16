package tex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strconv"
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
	// set env
	os.Setenv("LOG_FILE", "../../tmp/log.log")
	os.Setenv("ERR_FILE", "../../tmp/err.log")
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

func TestGetBalance(t *testing.T) {

	// .ledger.spot.<account_id>.<currency>.balance \ frozen \ available <-> redis核对
	ctx := context.Background()
	path, err := findLatestOmsSnapshot("../../tmp/snapshot")
	assert.NoError(t, err)
	ss := loadLedgerFrom(t, path)

	_, handler := apiconnect.NewTexServiceHandler(pkg.NewTexService())
	ts := httptest.NewServer(handler)
	defer ts.Close()

	client := apiconnect.NewTexServiceClient(ts.Client(), ts.URL)

	for acctID, balances := range ss {
		rsp, err := client.GetBalance(ctx, &connect.Request[api.GetBalanceReq]{Msg: &api.GetBalanceReq{
			AccountId: acctID,
		}})
		assert.NoError(t, err)
		assert.NotNil(t, rsp)

		for _, item := range rsp.Msg.Balances {
			balanceItem, ok := balances[item.Currency]
			assert.True(t, ok, "currency %s not found in snapshot", item.Currency)
			assert.Equal(t, balanceItem.Deposit, item.Balance, "currency %s balance mismatch", item.Currency)

			frozenFloat, err := strconv.ParseFloat(balanceItem.Frozen, 64)
			assert.NoError(t, err)

			availableFloat, err := strconv.ParseFloat(balanceItem.Deposit, 64)
			assert.NoError(t, err)

			availableFloat -= frozenFloat
			assert.Equal(t, fmt.Sprintf("%.8f", availableFloat), item.Available, "currency %s available mismatch", item.Currency)
			assert.Equal(t, balanceItem.Frozen, item.Frozen, "currency %s frozen mismatch", item.Currency)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
