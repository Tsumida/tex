package tex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
