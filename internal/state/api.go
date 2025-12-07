package state

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/go-redis/redis"
	"github.com/tsumida/lunaship/infra"
	"github.com/tsumida/tex/gen/api"
	"go.uber.org/zap"

	iutils "github.com/tsumida/tex/internal/utils"
)

// redis对象管理
type LuaExecutor[T any] struct {
	client       redis.UniversalClient
	LuaScript    string
	LuaScriptSha string

	mx sync.RWMutex // 保证线程安全
}

// APP初始化时调用
func (l *LuaExecutor[T]) PrepareLuaScript(ctx context.Context) error {
	if l.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	// load script into the configured redis client
	sha, err := l.client.ScriptLoad(l.LuaScript).Result()
	if err != nil {
		panic(fmt.Errorf("failed to load ledger redis script: %w", err))
	}
	l.LuaScriptSha = sha
	infra.GlobalLog().Info("Redis script loaded", zap.String("sha", l.LuaScriptSha))
	return nil
}

func (l *LuaExecutor[T]) fastPath(
	keys []string,
	scriptArgs ...any,
) error {
	if l.LuaScriptSha == "" {
		return fmt.Errorf("lua script sha is empty")
	}
	res, err := l.client.EvalSha(l.LuaScriptSha, keys, scriptArgs...).Result()
	if err == nil {
		if resInt, _ := res.(int64); resInt == 1 {
			infra.GlobalLog().Debug("updated redis", zap.Strings("key", keys))
		} else {
			infra.GlobalLog().Info("skipped outdated event", zap.Strings("key", keys))
		}
	}
	return err
}

func (l *LuaExecutor[T]) slowPathWithLock(
	ctx context.Context,
	keys []string,
	scriptArgs ...any,
) error {
	l.mx.Lock()
	defer l.mx.Unlock()
	// double check
	if err := l.fastPath(keys, scriptArgs...); err == nil {
		return nil
	}
	infra.GlobalLog().Warn("redis script not found, reloading", zap.String("sha", l.LuaScriptSha))
	if err := l.PrepareLuaScript(ctx); err != nil {
		infra.GlobalLog().Error("failed to reload lua script", zap.Error(err))
		return err
	}
	return l.fastPath(keys, scriptArgs...)
}

func (l *LuaExecutor[T]) UpdateOneEvent(
	ctx context.Context,
	keys []string,
	scriptArgs ...any,
) error {
	l.mx.RLock()
	err := l.fastPath(keys, scriptArgs...)
	l.mx.RUnlock()
	if err == nil {
		return nil
	}

	if strings.Contains(err.Error(), "NOSCRIPT") { // 修正了原代码中对 err 的检查
		return l.slowPathWithLock(ctx, keys, scriptArgs...)
	}
	return err
}

// 状态
//  1. balance:account_id:currency 		-> value=json(event)
//  2. balance_ts:account_id:currency 	-> value=event.update_time
//
// usage:
//
//	keys=["balance:123:USD", "balance_ts:123:USD"]
//	ARGV[1] = json(event)
//	ARGV[2] = event.update_time
func NewLedgerHandler(client redis.UniversalClient) *LuaExecutor[*api.BalanceEvent] {
	return &LuaExecutor[*api.BalanceEvent]{
		client: client,
		// lua脚本:
		// 1. 判断balance_ts:account_id:currency是否小于等于当前消息的ts
		// 2. 如果是, 则更新hash中的balance和balance_ts字段; 否则忽略
		LuaScript: `
local balance_key = KEYS[1]
local balance_ts_key = KEYS[2]

local current_ts = redis.call("GET", balance_ts_key)
if current_ts == false or tonumber(current_ts) <= tonumber(ARGV[2]) then
	redis.call("SET", balance_key, ARGV[1])
	redis.call("SET", balance_ts_key, ARGV[2])
	return 1
else
	return 0
end
`,
	}
}

// 状态
//  1. order_detail:order_id -> value=JSON(event)
//  2. active_order_list:account_id -> sorted set of order_id by tx_time
//  3. order_detail_ts:order_id -> value=event.tx_time, expire=8days
//
// usage:
//
//	keys=["orders:account_id"]
//	ARGV[1] = json(event)
//	ARGV[2] = event.tx_time
//	ARGV[3] = event.order_id
//	ARGV[4] = event.state
func NewOrderHandler(client redis.UniversalClient) *LuaExecutor[*api.OrderEvent] {
	return &LuaExecutor[*api.OrderEvent]{
		client: client,
		LuaScript: `
local order_list_key = KEYS[1]
local order_detail_key = "order_detail:" .. ARGV[3]

-- 如果订单时间戳是旧的, 则忽略
-- 检查订单状态, 如果订单是已完成/已取消状态, 则从active_order_list中移除
-- 更新订单详情和订单时间戳
local current_ts = redis.call("GET", "order_detail_ts:" .. ARGV[3])
if current_ts == false or tonumber(current_ts) <= tonumber(ARGV[2]) then
	-- 更新订单详情
	redis.call("SET", order_detail_key, ARGV[1])
	redis.call("SET", "order_detail_ts:" .. ARGV[3], ARGV[2])
	redis.call("EXPIRE", "order_detail_ts:" .. ARGV[3], 691200) -- 8 days

	-- 根据订单状态更新active_order_list
	if ARGV[4] == "COMPLETED" or ARGV[4] == "CANCELLED" then
		redis.call("ZREM", order_list_key, ARGV[3])
	else
		redis.call("ZADD", order_list_key, ARGV[2], ARGV[3])
	end

	return 1
else
	return 0
end

return 1
`,
	}
}

var (
	BarSec  = "1s"
	BarMin  = "1m"
	BarHour = "1h"
	BarDay  = "1d"
)

type KBar struct {
	// note: 由于数字币交易所没有开盘收盘，这里Open, Close实际上是一个聚合窗口的开始\最后价。
	Open        string `json:"open"`
	High        string `json:"high"`
	Low         string `json:"low"`
	Close       string `json:"close"`
	Volume      string `json:"volume"`
	StartInSec  uint64 `json:"start_in_sec"`
	StartInMin  uint64 `json:"start_in_min"`
	StartInHour uint64 `json:"start_in_hour"`
	StartInDay  uint64 `json:"start_in_day"`
}

// 只考虑成交
func KBarFromFillRecord(
	matchTimeInUs uint64,
	fill *api.FillRecord,
) (KBar, error) {

	var (
		sec  = matchTimeInUs / 1_000_000
		min  = sec / 60
		hour = min / 60
	)

	startInDay, err := iutils.GetDayStartTimeSec(int64(matchTimeInUs), iutils.EastEightZone)
	if err != nil {
		return KBar{}, fmt.Errorf("failed to get day start time: %w", err)
	}

	// todo: 考虑精度问题?
	return KBar{
		Open:        fill.Price,
		High:        fill.Price,
		Low:         fill.Price,
		Close:       fill.Price,
		Volume:      fill.Quantity,
		StartInSec:  sec * 1_000_000,
		StartInMin:  min * 60 * 1_000_000,
		StartInHour: hour * 3600 * 1_000_000,
		StartInDay:  uint64(startInDay),
	}, nil
}

type Notification struct {
	Type       string `json:"type"`       // "bar"
	Resolution string `json:"resolution"` // "SEC", "MIN", "HOUR", "DAY"
	MatchID    uint64 `json:"MatchID"`
	Data       any    `json:"data"`
}

// 状态
//  1. tick_<BASEQUOTE>:<match_id> -> value=LIST({...}), 一个tick直接对应一个FillRecord, 保留最新100条
//  2. kbar_<BASEQUOTE>:<interval>:<open_time> -> value=ZScoredSet(kbar), 	expire=30days // 30 days retention
//  3. kbar_match_id:<BASEQUOTE> -> value=last_match_id,
//  4. 如果tick会导致zscoredSet 最后一条k线数据更新, 则广播到 notification:<interval> channel
//
// usage:
//
//		keys=["SEC", "MIN", "HOUR", "DAY"]
//		ARGV[1] = match_id
//		ARGV[2] = sec_start_time
//		ARGV[3] = min_start_time
//		ARGV[4] = hour_start_time
//		ARGV[5] = day_start_time
//		ARGV[6] = open_price
//		ARGV[7] = high_price
//		ARGV[8] = low_price
//		ARGV[9] = close_price
//		ARGV[10] = quantity
//	 	ARGV[11] = BaseQuote
//	 	ARGV[12] = json(match_result)
func NewKBarUpdator(client redis.UniversalClient) *LuaExecutor[KBar] {
	return &LuaExecutor[KBar]{
		client: client,
		LuaScript: `
local function merge(existBar, newBar)
    existBar[3] = math.max(existBar[3], newBar[3]) -- 更新High Price
    existBar[4] = math.min(existBar[4], newBar[4]) -- 更新Low Price
    existBar[5] = newBar[5] -- close
    existBar[6] = existBar[6] + newBar[6] -- 更新quantity
end

local function notifyNewBar(barType, matchID, newBar)
	local topic = 'notification'
	redis.call('PUBLISH', topic, '{"type":"bar","resolution":"' .. barType .. '","matchID":' .. matchID .. ',"data":' .. cjson.encode(newBar) .. '}')
end

local function tryMergeLast(barType, matchID, zsetBars, timestamp, newBar)
    local topic = 'notification'
    local popedScore, popedBar
    -- 查找最后一个Bar:
    local poped = redis.call('ZPOPMAX', zsetBars)
    if #poped == 0 then
        -- ZScoredSet无任何bar, 直接添加:
        redis.call('ZADD', zsetBars, timestamp, cjson.encode(newBar))
        notifyNewBar(barType, matchID, newBar)
    else
        popedBar = cjson.decode(poped[1])
        popedScore = tonumber(poped[2])
        if popedScore == timestamp then
            -- 合并Bar并发送通知:
            merge(popedBar, newBar)
            redis.call('ZADD', zsetBars, popedScore, cjson.encode(popedBar))
            notifyNewBar(barType, matchID, popedBar)
        else
            -- 可持久化最后一个Bar，生成新的Bar:
            if popedScore < timestamp then
                redis.call('ZADD', zsetBars, popedScore, cjson.encode(popedBar), timestamp, cjson.encode(newBar))
                notifyNewBar(barType, matchID, newBar)
                return popedBar
            end
        end
    end
    return nil
end


-- 维护最近200条match_result
redis.call("LPUSH", match_result_key, ARGV[3])
redis.call("LTRIM", match_result_key, 0, 199)

-- 检查是否有新的match_id
local last_match_id = redis.call("GET", last_match_id_key)
if last_match_id == false or tonumber(last_match_id) < tonumber(match_id) then
	redis.call("SET", last_match_id_key, match_id)

	-- 更新K线数据
	tickKey = "tick_" .. ARGV[11]
	zsetBars = { KEYS[2], KEYS[3], KEYS[4], KEYS[5] }
    barTypeStartTimes = { tonumber(ARGV[2]), tonumber(ARGV[3]), tonumber(ARGV[4]), tonumber(ARGV[5]) }
    openPrice = tonumber(ARGV[6])
    highPrice = tonumber(ARGV[7])
    lowPrice = tonumber(ARGV[8])
    closePrice = tonumber(ARGV[9])
    quantity = tonumber(ARGV[10])
	match_result_json = ARGV[12]

	-- 缓存最近100个tick
	local persistBars = {}
	redis.call("LPUSH", tickKey, match_result_json)
	redis.call("LTRIM", tickKey, 0, 99)

	-- 
    local i, bar
    local names = { 'SEC', 'MIN', 'HOUR', 'DAY' }
    -- 检查是否可以merge:
    for i = 1, 4 do
        bar = tryMergeLast(names[i], seqId, zsetBars[i], barTypeStartTimes[i], { barTypeStartTimes[i], openPrice, highPrice, lowPrice, closePrice, quantity })
        if bar then
            persistBars[names[i]] = bar
        end
    end
    redis.call('SET', KEY_BAR_SEQ, seqId)
    return cjson.encode(persistBars)

end
return '{}'
`,
	}
}
