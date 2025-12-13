package pkg

import (
	"encoding/json"
	"fmt"

	"github.com/IBM/sarama"
	"github.com/tsumida/lunaship/infra"
	"github.com/tsumida/tex/gen/api"
	"github.com/tsumida/tex/pkg/kafka"
	redisstate "github.com/tsumida/tex/pkg/state/redis_state"
	"go.uber.org/zap"
)

// HandleLedgerEvent writes latest account balance snapshots into Redis.
func HandleLedgerEvent(l redisstate.LuaExecutorAPI) kafka.MsgHandlerFunc {

	return func(s sarama.ConsumerGroupSession, msg *sarama.ConsumerMessage) error {
		logger := infra.GlobalLog().With(
			zap.String("handler", "HandleLedgerEvent"),
			zap.String("topic", msg.Topic),
			zap.Int32("partition", msg.Partition),
			zap.Int64("offset", msg.Offset))

		defer func() {
			s.MarkMessage(msg, "")
			s.Commit()
		}()

		if len(msg.Value) == 0 {
			logger.Warn("skip empty ledger message")
			return nil
		}
		return handleLedgerEvent(logger, l, s, msg.Value)
	}
}

func handleLedgerEvent(
	logger *zap.Logger,
	l redisstate.LuaExecutorAPI,
	s sarama.ConsumerGroupSession,
	data []byte,
) error {
	event, err := (&MsgDecoder{}).DecodeLedgerEvent(data)
	if err != nil {
		logger.Error("failed to decode ledger event", zap.Error(err))
		return err
	}

	for _, event := range event.Events {
		jsonBuf, err := json.Marshal(event)
		if err != nil {
			logger.Error("failed to marshal ledger event to JSON", zap.Error(err))
			continue
		}
		logger.Info("recv ledger_event", zap.String("payload", string(jsonBuf)))
		keys := []string{
			fmt.Sprintf("balance:%d:%s", event.AccountId, event.Currency),
			fmt.Sprintf("balance_ts:%d:%s", event.AccountId, event.Currency),
		}

		if err := l.UpdateOneEvent(s.Context(), keys, string(jsonBuf), event.UpdateTime); err != nil {
			logger.Error("failed to update ledger event in Redis", zap.Error(err))
			continue
		}
	}
	return nil
}

// HandleOrderEvent keeps per-account order list in Redis sorted by tx_time.
func HandleOrderEvent(l redisstate.LuaExecutorAPI) kafka.MsgHandlerFunc {
	redisState := redisstate.NewOrderData()
	return func(s sarama.ConsumerGroupSession, msg *sarama.ConsumerMessage) error {
		logger := infra.GlobalLog().With(
			zap.String("handler", "HandleOrderEvent"),
			zap.String("topic", msg.Topic),
			zap.Int32("partition", msg.Partition),
			zap.Int64("offset", msg.Offset))

		defer func() {
			s.MarkMessage(msg, "")
			s.Commit()
		}()

		if len(msg.Value) == 0 {
			logger.Warn("skip empty order message")
			return nil
		}
		return handleOrderEvent(logger, l, s, redisState, msg.Value)
	}
}

// 对每条FillOrder执行lua脚本
func handleOrderEvent(
	logger *zap.Logger,
	l redisstate.LuaExecutorAPI,
	s sarama.ConsumerGroupSession,
	rs *redisstate.Order,
	data []byte,
) error {
	event, err := (&MsgDecoder{}).DecodeOrderEvent(data)
	if err != nil {
		logger.Error("failed to decode order event", zap.Error(err))
		return err
	}

	for _, event := range event.Events {
		jsonBuf, err := json.Marshal(event)
		if err != nil {
			logger.Error("marshal order event", zap.Error(err))
			continue
		}
		logger.Info("recv order_event", zap.String("payload", string(jsonBuf)))
		keys := []string{
			rs.OrderDetailKey(event.OrderId),
			rs.OrderDetailTsKey(event.OrderId),
			rs.OrderListKey(event.AccountId),
		}
		orderStateStr := api.OrderState_name[int32(event.OrderState)]
		if err := l.UpdateOneEvent(s.Context(), keys, string(jsonBuf), event.TxTime, event.OrderId, orderStateStr); err != nil {
			logger.Error("failed to update order event in Redis", zap.Error(err))
			continue
		}
	}
	return nil
}

// HandleMatchResultEvent appends raw match result payloads into Redis for K-Bar building.
func HandleMatchResultEvent(l redisstate.LuaExecutorAPI) kafka.MsgHandlerFunc {

	redisState := redisstate.NewKBarData()

	return func(s sarama.ConsumerGroupSession, msg *sarama.ConsumerMessage) error {
		logger := infra.GlobalLog().With(
			zap.String("handler", "HandleMatchResultEvent"),
			zap.String("topic", msg.Topic),
			zap.Int32("partition", msg.Partition),
			zap.Int64("offset", msg.Offset))

		defer func() {
			s.MarkMessage(msg, "")
			s.Commit()
		}()

		if len(msg.Value) == 0 {
			logger.Warn("skip empty match result message")
			return nil
		}
		return handleMatchResultEvent(logger, l, s, msg.Value, redisState, uint64(msg.Timestamp.UnixMicro()))
	}
}

func handleMatchResultEvent(
	logger *zap.Logger,
	l redisstate.LuaExecutorAPI,
	s sarama.ConsumerGroupSession,
	data []byte,
	rs *redisstate.KBarData,
	ts uint64, // us
) error {
	batch, err := (&MsgDecoder{}).DecodeBatchMatchResult(data)
	if err != nil {
		logger.Error("failed to decode match result event", zap.Error(err))
		return err
	}

	for _, result := range batch.Results {
		if result.Action != api.BizAction_FillOrder {
			logger.Debug("skipping non-fill match result", zap.Int32("action", int32(result.Action)))
			continue
		}
		for _, fillRecord := range result.Records {
			p := fillRecord.TradePair
			jsonBuf, err := json.MarshalIndent(fillRecord, "", "  ")
			if err != nil {
				logger.Error("failed to marshal match result event to JSON", zap.Error(err))
				continue
			}
			logger.Info("recv match_result_event", zap.String("payload", string(jsonBuf)))

			keys := rs.KeyFns(p.Base, p.Quote)
			pair := fmt.Sprintf("%s%s", p.Base, p.Quote)

			// todo: use match time instead of process time
			// ts := uint64(msg.Timestamp.UnixMicro())
			kbar, err := redisstate.KBarFromFillRecord(ts, fillRecord)
			if err != nil {
				logger.Error("failed to create K-Bar from fill record", zap.Error(err))
				continue
			}

			// 入参见 NewKBarUpdator()
			if err := l.UpdateOneEvent(
				s.Context(),
				keys,
				fillRecord.MatchId,
				kbar.WindowStartInSec,
				kbar.WindowStartInMin,
				kbar.WindowStartInHour,
				kbar.WindowStartInDay,
				kbar.Open,
				kbar.High,
				kbar.Low,
				kbar.Close,
				kbar.Volume,
				pair,
				string(jsonBuf),
			); err != nil {
				logger.Error("failed to update K-Bar in Redis", zap.Error(err))
				continue
			}
		}
	}

	return nil
}

func orderDetailFromEvent(
	event *api.OrderEvent,
) *api.OrderDetail {
	return &api.OrderDetail{
		Original: &api.Order{
			OrderId: event.OrderId,
			TradePair: &api.TradePair{
				Base:  event.Base,
				Quote: event.Quote,
			},
			Direction:     event.Direction,
			TimeInForce:   api.TimeInForce_value[event.TimeInForce],
			OrderType:     api.OrderType_value[event.OrderType],
			Price:         event.Price,
			Quantity:      event.TargetQty,
			CreateTime:    event.CreateTime,
			ClientOrderId: event.ClientOrderId,
			StpStrategy:   api.STPStrategy_value[event.StpStrategy],
			AccountId:     event.AccountId,
			PostOnly:      event.PostOnly,
			TradeId:       event.TradeId,
			PrevTradeId:   event.PrevTradeId,
		},
		CurrentState:   api.OrderState_name[event.OrderState],
		FilledQuantity: event.FilledQty,
		LastTradeId:    event.TradeId,
		UpdateTime:     event.TxTime,
	}
}

func balanceFromBalanceEvent(
	event *api.BalanceEvent,
) *api.BalanceItem {
	return &api.BalanceItem{
		Currency:   event.Currency,
		Balance:    event.Balance,
		Available:  event.Deposit,
		Frozen:     event.Frozen,
		UpdateTime: event.UpdateTime,
	}
}
