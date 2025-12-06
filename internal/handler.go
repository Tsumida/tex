package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"github.com/tsumida/lunaship/infra"
	"github.com/tsumida/tex/gen/api"
	"github.com/tsumida/tex/internal/kafka"
	"github.com/tsumida/tex/internal/state"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

// MsgDecoder provides helpers to deserialize kafka message payloads.
type MsgDecoder struct{}

func (d *MsgDecoder) DecodeBatchMatchResult(msg *sarama.ConsumerMessage) (*api.BatchMatchResult, error) {
	if msg == nil || len(msg.Value) == 0 {
		return nil, errors.New("empty kafka message for MatchResult")
	}
	out := &api.BatchMatchResult{}
	if err := proto.Unmarshal(msg.Value, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (d *MsgDecoder) DecodeLedgerEvent(msg *sarama.ConsumerMessage) (*api.BatchBalanceEvent, error) {
	if msg == nil || len(msg.Value) == 0 {
		return nil, errors.New("empty kafka message for BalanceEvent")
	}
	out := &api.BatchBalanceEvent{}
	if err := proto.Unmarshal(msg.Value, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (d *MsgDecoder) DecodeOrderEvent(msg *sarama.ConsumerMessage) (*api.BatchOrderEvent, error) {
	if msg == nil || len(msg.Value) == 0 {
		return nil, errors.New("empty kafka message for OrderEvent")
	}
	out := &api.BatchOrderEvent{}
	if err := proto.Unmarshal(msg.Value, out); err != nil {
		return nil, err
	}
	return out, nil
}

// HandleLedgerEvent writes latest account balance snapshots into Redis.
func HandleLedgerEvent(l state.LuaExecutor[*api.BalanceEvent]) kafka.MsgHandlerFunc {

	return func(s sarama.ConsumerGroupSession, msg *sarama.ConsumerMessage) error {
		logger := infra.GlobalLog().With(
			zap.String("handler", "HandleLedgerEvent"),
			zap.String("topic", msg.Topic),
			zap.Int32("partition", msg.Partition),
			zap.Int64("offset", msg.Offset))

		defer s.MarkMessage(msg, "")
		event, err := (&MsgDecoder{}).DecodeLedgerEvent(msg)
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
			keys := []string{
				fmt.Sprintf("balance:%d:%s", event.AccountId, event.Currency),
				fmt.Sprintf("balance_ts:%d:%s", event.AccountId, event.Currency),
			}
			if err := l.UpdateOneEvent(s.Context(), event, keys, string(jsonBuf), event.UpdateTime); err != nil {
				logger.Error("failed to update ledger event in Redis", zap.Error(err))
				continue
			}
		}
		return nil
	}
}

// HandleOrderEvent keeps per-account order list in Redis sorted by tx_time.
func HandleOrderEvent(l state.LuaExecutor[*api.OrderEvent]) kafka.MsgHandlerFunc {
	return func(s sarama.ConsumerGroupSession, msg *sarama.ConsumerMessage) error {
		logger := infra.GlobalLog().With(
			zap.String("handler", "HandleOrderEvent"),
			zap.String("topic", msg.Topic),
			zap.Int32("partition", msg.Partition),
			zap.Int64("offset", msg.Offset))

		defer s.MarkMessage(msg, "")
		event, err := (&MsgDecoder{}).DecodeOrderEvent(msg)
		if err != nil {
			logger.Error("failed to decode order event", zap.Error(err))
			return err
		}

		for _, event := range event.Events {

			jsonBuf, err := json.Marshal(event)
			if err != nil {
				panic(err)
			}
			keys := []string{
				fmt.Sprintf("orders:%d", event.AccountId),
			}
			// todo: 以msg为准，补全tx_time
			ts := event.TxTime
			if ts == 0 {
				ts = uint64(time.Now().UnixMicro())
			}

			if err := l.UpdateOneEvent(s.Context(), event, keys, string(jsonBuf), ts, event.OrderId, event.OrderState); err != nil {
				logger.Error("failed to update order event in Redis", zap.Error(err))
				continue
			}
		}
		return nil
	}
}

// HandleMatchResultEvent appends raw match result payloads into Redis for K-Bar building.
func HandleMatchResultEvent(l state.LuaExecutor[*api.MatchResult]) kafka.MsgHandlerFunc {
	return func(s sarama.ConsumerGroupSession, msg *sarama.ConsumerMessage) error {
		defer s.MarkMessage(msg, "")
		event, err := (&MsgDecoder{}).DecodeBatchMatchResult(msg)
		if err != nil {
			panic(err)
		}

		jsonBuf, err := json.MarshalIndent(event, "", "  ")
		if err != nil {
			panic(err)
		}
		fmt.Printf("%s\n", jsonBuf)

		return nil
	}
}
