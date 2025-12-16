package redisstate

import (
	"fmt"

	"github.com/tsumida/tex/gen/api"

	"github.com/tsumida/lunaship/redis"
	iutils "github.com/tsumida/tex/pkg/utils"

	v9 "github.com/redis/go-redis/v9"
)

// 状态
//  1. balance:account_id:currency 		-> value=json(event)
//  2. balance_ts:account_id:currency 	-> value=event.update_time
//
// usage:
//
//	keys=["balance:123:USD", "balance_ts:123:USD"]
//	ARGV[1] = json(event)
//	ARGV[2] = event.update_time
func NewLedgerHandler(client v9.UniversalClient) *redis.LuaExecutor {
	return redis.NewLuaExecutorWithLogger(
		"LedgerHandler",
		client,
		// lua脚本:
		// 1. 判断balance_ts:account_id:currency是否小于等于当前消息的ts
		// 2. 如果是, 则更新hash中的balance和balance_ts字段; 否则忽略
		`
local balance_key = KEYS[1]
local balance_ts_key = KEYS[2]

local current_ts = redis.call("GET", balance_ts_key)
if current_ts == false or tonumber(current_ts) <= tonumber(ARGV[2]) then
	redis.call("SET", balance_key, ARGV[1])
	redis.call("SET", balance_ts_key, ARGV[2])
	redis.log(redis.LOG_NOTICE, "Updated balance for key " .. balance_key)
	return 1
else
	redis.log(redis.LOG_NOTICE, "Skipped outdated balance for key, current_ts=" .. current_ts .. ", new_ts=" .. tostring(ARGV[2]))
	return 0
end
`,
		nil,
		nil,
	)
}

// 状态
//  1. order_detail:order_id -> value=JSON(event)
//  2. order_detail_ts:order_id -> value=event.tx_time, expire=8days
//  3. orders:account_id: -> value=ZScoredSet(key=tx_time, value=order_id)
//
// usage:
//
//	keys=["order_detail:order_id", "order_detail_ts:order_id", "orders:account_id"]
//	ARGV[1] = json(event)
//	ARGV[2] = event.update_time
//	ARGV[3] = event.order_id
//	ARGV[4] = event.state
func NewOrderHandler(client v9.UniversalClient) *redis.LuaExecutor {
	return redis.NewLuaExecutorWithLogger(
		"OrderHandler",
		client,
		`
local order_detail_key = KEYS[1]
local order_detail_ts_key = KEYS[2]
local order_list_key = KEYS[3]


-- 如果订单时间戳是旧的, 则忽略
-- 检查订单状态, 如果订单是已完成/已取消状态, 则从active_order_list中移除
-- 更新订单详情和订单时间戳
local current_ts = redis.call("GET", order_detail_ts_key)
if current_ts == false or tonumber(current_ts) <= tonumber(ARGV[2]) then
	-- 更新订单详情
	redis.call("SET", order_detail_key, ARGV[1])
	redis.call("SET", order_detail_ts_key, ARGV[2])

	redis.call("ZADD", order_list_key, tonumber(ARGV[2]), ARGV[3])

	return 1
else
	redis.log(redis.LOG_NOTICE, "Skipped outdated order for key, current_ts=" .. current_ts .. ", new_ts=" .. tostring(ARGV[2]))
	return 0
end
`, nil, nil)
}

// 状态
//  1. orders:account_id -> ZScoredSet(key=tx_time, value=order_id)
//  2. order_detail:order_id -> value=JSON(event)
//
// usage:
//
//	keys=["orders:account_id"]
//	ARGV[1] = page_offset
//	ARGV[2] = page_limit
//
//	返回  []json(order_detail)
func NewOrderListHandler(client v9.UniversalClient, respFunc func(data any) error) *redis.LuaExecutor {
	return redis.NewLuaExecutorWithLogger(
		"OrderLister",
		client, `
local order_list_key = KEYS[1]
local order_detail_prefix = "order_detail:"

local page_offset = tonumber(ARGV[1])
local page_limit = tonumber(ARGV[2])

-- 获取订单ID列表
local order_ids = redis.call("ZREVRANGE", order_list_key, page_offset, page_offset + page_limit - 1)
local order_details = {}

for i, order_id in ipairs(order_ids) do
	local order_detail_key = order_detail_prefix .. order_id
	local order_detail = redis.call("GET", order_detail_key)
	if order_detail then
		table.insert(order_details, order_detail)
	end
end

return order_details
`,
		respFunc, nil)
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

	WindowStartInDay, err := iutils.GetDayStartTimeSec(int64(matchTimeInUs), iutils.EastEightZone)
	if err != nil {
		return KBar{}, fmt.Errorf("failed to get day start time: %w", err)
	}

	// todo: 考虑精度问题?
	return KBar{
		Open:              fill.Price,
		High:              fill.Price,
		Low:               fill.Price,
		Close:             fill.Price,
		Volume:            fill.Quantity,
		WindowStartInSec:  sec,
		WindowStartInMin:  min * 60,
		WindowStartInHour: hour * 3600,
		WindowStartInDay:  uint64(WindowStartInDay),
	}, nil
}

type Notification struct {
	Type       string `json:"type"`       // "bar"
	Resolution string `json:"resolution"` // "SEC", "MIN", "HOUR", "DAY"
	MatchID    uint64 `json:"MatchID"`
	Data       any    `json:"data"`
}

// 状态
//  1. tick:<BASEQUOTE>:<match_id> -> value=LIST({...}), 一个tick直接对应一个FillRecord, 保留最新100条
//  2. kbar:<BASEQUOTE>:<interval> -> value=ZScoredSet(kbar),
//  3. kbar_match_id:<BASEQUOTE> -> value=last_match_id,
//  4. 如果tick会导致zscoredSet 最后一条k线数据更新, 则广播到 notification:<interval> channel
//
// usage:
//
//		keys=["tick:<PAIR>","kbar:<PAIR>:SEC", "kbar:<PAIR>:MIN", "kbar:<PAIR>:HOUR", "kbar:<PAIR>:DAY"]
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
//	 	ARGV[12] = json(fill_record)
func NewKBarUpdator(client v9.UniversalClient) *redis.LuaExecutor {
	return redis.NewLuaExecutorWithLogger(
		"KBarUpdator",
		client,
		`
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
		return 1
    else
        popedBar = cjson.decode(poped[1])
		
        popedScore = tonumber(poped[2])
        if popedScore == timestamp then
            -- 合并Bar并发送通知:
            merge(popedBar, newBar)
            redis.call('ZADD', zsetBars, popedScore, cjson.encode(popedBar))
            notifyNewBar(barType, matchID, popedBar)
			redis.log(redis.LOG_NOTICE, "Merged bar for key " .. zsetBars)
			return 1
        else
            -- 可持久化最后一个Bar，生成新的Bar:
            if popedScore < timestamp then
                redis.call('ZADD', zsetBars, popedScore, cjson.encode(popedBar), timestamp, cjson.encode(newBar))
                notifyNewBar(barType, matchID, newBar)
				redis.log(redis.LOG_NOTICE, "Added new bar for key " .. zsetBars)
                return 1
            else
				-- 时间戳异常, 放回原数据
				redis.call('ZADD', zsetBars, popedScore, cjson.encode(popedBar))
				redis.log(redis.LOG_NOTICE, "Skipped outdated bar for key " .. zsetBars .. ", current_ts=" .. popedScore .. ", new_ts=" .. timestamp)
				return 1
			end
        end
    end
    return 0
end


-- 初始化参数
local match_id = ARGV[1]
local persistBars = {}
local last_match_id_key = "kbar_match_id:" .. ARGV[11]
local match_result_key = KEYS[1]


-- 检查是否有新的match_id
local last_match_id = redis.call("GET", last_match_id_key)
if last_match_id == false or tonumber(last_match_id) < tonumber(match_id) then

	-- 更新K线数据
	local zsetBars = { KEYS[2], KEYS[3], KEYS[4], KEYS[5] }
    local barTypeStartTimes = { tonumber(ARGV[2]), tonumber(ARGV[3]), tonumber(ARGV[4]), tonumber(ARGV[5]) }

	-- 注意float转换问题. 但price一般都是小数点后2位
    local openPrice = tonumber(ARGV[6])
    local highPrice = tonumber(ARGV[7])
    local lowPrice = tonumber(ARGV[8])
    local closePrice = tonumber(ARGV[9])

	-- todo: quantity的精度问题
    local quantity = tonumber(ARGV[10])
	local match_result_json = ARGV[12]

	
	-- 维护最近100条match_result
	redis.call("LPUSH", match_result_key, ARGV[12])
	redis.call("LTRIM", match_result_key, 0, 99)

	-- 
    local i, bar
    local names = { 'SEC', 'MIN', 'HOUR', 'DAY' }
    -- 检查是否可以merge:
    for i = 1, 4 do
        local bar = tryMergeLast(names[i], match_id, zsetBars[i], barTypeStartTimes[i], { barTypeStartTimes[i], openPrice, highPrice, lowPrice, closePrice, quantity })
        if bar then
            persistBars[names[i]] = bar
        end
    end
	redis.call("SET", last_match_id_key, match_id)
	redis.log(redis.LOG_NOTICE, "Updated bars for key " .. last_match_id_key)
    return 1
else
	redis.log(redis.LOG_NOTICE, "Skipped outdated match " .. last_match_id_key .. ", current_id=" .. tostring(last_match_id) .. ", new_id=" .. tostring(match_id))
	return 0
end
`, nil, nil)
}
