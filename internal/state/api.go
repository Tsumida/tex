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
	event T,
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
// example:
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
//  1. order_detail:order_id -> value=json(order_event), expire=14days // 14 days retention
//  2. order_list:account_id -> sorted set of order_id by tx_time
//
// example:
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

-- Store order detail with expiration
redis.call("SET", order_detail_key, ARGV[1])
redis.call("EXPIRE", order_detail_key, 1296000) -- 15 days in seconds

-- Update sorted set of orders
redis.call("ZADD", order_list_key, ARGV[2], ARGV[3])

return 1
`,
	}
}

// todo: review
func NewKBarUpdator(client redis.UniversalClient) *LuaExecutor[*api.MatchResult] {
	return &LuaExecutor[*api.MatchResult]{
		client: client,
		LuaScript: `
return 1
`,
	}
}
