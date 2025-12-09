package redisstate

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

// Require: Thread-safe
type LuaExecutorAPI interface {
	// APP初始化时调用, 预加载lua脚本并记录sha，后续调用都通过EvalSha执行，降低网络传输开销
	PrepareLuaScript(ctx context.Context) error

	// 通过EvalSha执行lua脚本更新redis状态。如果发现脚本不存在，尝试重新加载脚本
	UpdateOneEvent(
		ctx context.Context,
		keys []string,
		scriptArgs ...any,
	) error
}

// redis对象管理
type LuaExecutor[T any] struct {
	name         string
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
	infra.GlobalLog().Info("Redis script loaded", zap.String("name", l.name), zap.String("sha", l.LuaScriptSha))
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
	infra.GlobalLog().Debug("exec lua script", zap.Strings("keys", keys), zap.Any("args", scriptArgs))
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

var _ LuaExecutorAPI = (*LuaExecutor[*api.BalanceEvent])(nil)
var _ LuaExecutorAPI = (*LuaExecutor[*api.OrderEvent])(nil)
var _ LuaExecutorAPI = (*LuaExecutor[KBar])(nil)

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
		name:   "LedgerHandler",
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
	redis.log(redis.LOG_NOTICE, "Updated balance for key " .. balance_key)
	return 1
else
	redis.log(redis.LOG_NOTICE, "Skipped outdated balance for key " .. balance_key)
	return 0
end
`,
	}
}

// 状态
//  1. order_detail:order_id -> value=JSON(event)
//  2. order_detail_ts:order_id -> value=event.tx_time, expire=8days
//
// usage:
//
//	keys=["order_detail:order_id"]
//	ARGV[1] = json(event)
//	ARGV[2] = event.tx_time
//	ARGV[3] = event.order_id
//	ARGV[4] = event.state
func NewOrderHandler(client redis.UniversalClient) *LuaExecutor[*api.OrderEvent] {
	return &LuaExecutor[*api.OrderEvent]{
		name:   "OrderHandler",
		client: client,
		LuaScript: `
local order_detail_key = KEYS[1]

-- 如果订单时间戳是旧的, 则忽略
-- 检查订单状态, 如果订单是已完成/已取消状态, 则从active_order_list中移除
-- 更新订单详情和订单时间戳
local current_ts = redis.call("GET", "order_detail_ts:" .. ARGV[3])
if current_ts == false or tonumber(current_ts) <= tonumber(ARGV[2]) then
	-- 更新订单详情
	redis.call("SET", order_detail_key, ARGV[1])
	redis.call("SET", "order_detail_ts:" .. ARGV[3], ARGV[2])
	redis.call("EXPIRE", "order_detail_ts:" .. ARGV[3], 691200) -- 8 days
	return 1
else
	redis.log(redis.LOG_NOTICE, "Skipped outdated order for key " .. order_detail_key)
	return 0
end
`,
	}
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
//  1. tick_<BASEQUOTE>:<match_id> -> value=LIST({...}), 一个tick直接对应一个FillRecord, 保留最新100条
//  2. kbar:<BASEQUOTE>:<interval> -> value=ZScoredSet(kbar),
//  3. kbar_match_id:<BASEQUOTE> -> value=last_match_id,
//  4. 如果tick会导致zscoredSet 最后一条k线数据更新, 则广播到 notification:<interval> channel
//
// usage:
//
//		keys=["kbar:<PAIR>:SEC", "kbar:<PAIR>:MIN", "kbar:<PAIR>:HOUR", "kbar:<PAIR>:DAY"]
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
func NewKBarUpdator(client redis.UniversalClient) *LuaExecutor[KBar] {
	return &LuaExecutor[KBar]{
		name:   "KBarUpdator",
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
				redis.log(redis.LOG_NOTICE, "Skipped outdated bar for key " .. zsetBars)
				return 0
			end
        end
    end
    return 0
end


-- 初始化参数
local match_id = ARGV[1]
local persistBars = {}
local last_match_id_key = "kbar_match_id:" .. ARGV[11]
local match_result_key = "tick_" .. ARGV[11]


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
        bar = tryMergeLast(names[i], match_id, zsetBars[i], barTypeStartTimes[i], { barTypeStartTimes[i], openPrice, highPrice, lowPrice, closePrice, quantity })
        if bar then
            persistBars[names[i]] = bar
        end
    end
	redis.call("SET", last_match_id_key, match_id)
	redis.log(redis.LOG_NOTICE, "Updated bars for key " .. last_match_id_key)
    return cjson.encode(persistBars)
end

redis.log(redis.LOG_NOTICE, "Skipped outdated balance for key " .. balance_key)
return '{}'
`,
	}
}
