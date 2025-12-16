package tex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

func texClient() apiconnect.TexServiceClient {
	_, handler := apiconnect.NewTexServiceHandler(pkg.NewTexService())
	ts := httptest.NewServer(handler)
	client := apiconnect.NewTexServiceClient(ts.Client(), "http://mvp_tex:8180")
	return client
}
func TestMain(m *testing.M) {
	// set env
	os.Setenv("LOG_FILE", "../../tmp/log.log")
	os.Setenv("ERR_FILE", "../../tmp/err.log")
	// go pkg.RunApp(context.TODO())
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

	client := texClient()
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

	client := texClient()
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

	client := texClient()
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
	path, err := findLatestOmsSnapshot("../../../tmp/snapshot")
	assert.NoError(t, err)
	ss := loadLedgerFrom(t, path)

	client := texClient()
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

type BalanceItem struct {
	// {
	// 	"account_id": 1000,
	// 	"currency": "ETH",
	// 	"deposit": "-2000",
	// 	"frozen": "0",
	// 	"update_time": 0
	// }
	Currency   string `json:"currency"`
	Deposit    string `json:"deposit"`
	Frozen     string `json:"frozen"`
	UpdateTime int64  `json:"update_time"`
}

type BalanceMap map[uint64]map[string]BalanceItem

// findLatestOmsSnapshot 查找目录下最新的匹配 oms_snapshot*.json 的文件
func findLatestOmsSnapshot(dirPath string) (string, error) {
	// 读取目录内容
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return "", fmt.Errorf("读取目录 %s 失败: %w", dirPath, err)
	}

	var latestFilePath string
	var latestTime time.Time

	// 遍历目录项
	for _, entry := range entries {
		// 跳过目录
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()

		// 检查文件名是否匹配 oms_snapshot*.json 模式
		if strings.HasPrefix(fileName, "oms_snapshot") && strings.HasSuffix(fileName, ".json") {
			// 获取文件信息
			info, err := entry.Info()
			if err != nil {
				// 记录错误但不中断，继续处理其他文件
				fmt.Printf("获取文件 %s 信息失败: %v\n", fileName, err)
				continue
			}

			// 获取文件的修改时间
			modTime := info.ModTime()

			// 比较时间，找到最新的文件
			if latestFilePath == "" || modTime.After(latestTime) {
				latestTime = modTime
				latestFilePath = filepath.Join(dirPath, fileName)
			}
		}
	}

	if latestFilePath == "" {
		return "", fmt.Errorf("在目录 %s 中未找到匹配的文件", dirPath)
	}

	return latestFilePath, nil
}

func loadLedgerFrom(t *testing.T, snapshotPath string) BalanceMap {
	t.Helper()
	// 加载snapshots下最新一个oms_snapshot*.json文件
	data, err := os.ReadFile(snapshotPath)
	assert.NoError(t, err)
	rawData := make(map[string]any)
	err = json.Unmarshal(data, &rawData)
	assert.NoError(t, err)
	// mapper = .ledger.spots.<account_id>.<currency>
	ledgerData, ok := rawData["ledger"].(map[string]any)["spots"].(map[string]any)
	assert.True(t, ok)

	balanceMap := make(BalanceMap)
	for accIDStr, v := range ledgerData {
		accID, err := strconv.ParseUint(accIDStr, 10, 64)
		assert.NoError(t, err)
		currencyMap, ok := v.(map[string]any)
		assert.True(t, ok)
		balanceMap[accID] = make(map[string]BalanceItem)
		for currency, item := range currencyMap {
			itemBytes, err := json.Marshal(item)
			assert.NoError(t, err)
			var balanceItem BalanceItem
			err = json.Unmarshal(itemBytes, &balanceItem)
			assert.NoError(t, err)
			balanceMap[accID][currency] = balanceItem
		}
	}
	return balanceMap
}
