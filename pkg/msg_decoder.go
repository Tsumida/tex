package pkg

import (
	"github.com/tsumida/tex/gen/api"
	"google.golang.org/protobuf/proto"
)

// MsgDecoder provides helpers to deserialize kafka message payloads.
type MsgDecoder struct{}

func (d *MsgDecoder) DecodeBatchMatchResult(value []byte) (*api.BatchMatchResult, error) {
	out := &api.BatchMatchResult{}
	if err := proto.Unmarshal(value, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (d *MsgDecoder) DecodeLedgerEvent(value []byte) (*api.BatchBalanceEvent, error) {
	out := &api.BatchBalanceEvent{}
	if err := proto.Unmarshal(value, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (d *MsgDecoder) DecodeOrderEvent(value []byte) (*api.BatchOrderEvent, error) {
	out := &api.BatchOrderEvent{}
	if err := proto.Unmarshal(value, out); err != nil {
		return nil, err
	}
	return out, nil
}
