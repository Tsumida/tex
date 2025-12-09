package internal

import (
	"encoding/json"
	"fmt"

	"github.com/IBM/sarama"
	"github.com/tsumida/lunaship/infra"
	"github.com/tsumida/tex/gen/api"
	"github.com/tsumida/tex/internal/kafka"
	redisstate "github.com/tsumida/tex/internal/state/redis_state"
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
		return handleLedgerEvent(logger, l, s, msg.Value, uint64(msg.Timestamp.UnixMicro()))
	}
}

func handleLedgerEvent(
	logger *zap.Logger,
	l redisstate.LuaExecutorAPI,
	s sarama.ConsumerGroupSession,
	data []byte,
	ts uint64,
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

		// todo: 以msg为准，补全tx_time
		if event.UpdateTime == 0 {
			event.UpdateTime = ts
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
		return handleOrderEvent(logger, l, s, redisState, msg.Value, uint64(msg.Timestamp.UnixMicro()))
	}
}

// 对每条FillOrder执行lua脚本
func handleOrderEvent(
	logger *zap.Logger,
	l redisstate.LuaExecutorAPI,
	s sarama.ConsumerGroupSession,
	rs *redisstate.Order,
	data []byte,
	ts uint64,
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
		}
		txTime := event.TxTime
		if txTime == 0 {
			txTime = ts
		}
		orderStateStr := api.OrderState_name[int32(event.OrderState)]
		if err := l.UpdateOneEvent(s.Context(), keys, string(jsonBuf), txTime, event.OrderId, orderStateStr); err != nil {
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
