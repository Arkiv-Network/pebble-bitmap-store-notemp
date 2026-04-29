package main

import (
	"fmt"

	"github.com/Arkiv-Network/arkiv-events/events"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// CommitChainRequest is the input shape for arkiv_commitChain.
type CommitChainRequest struct {
	Blocks []ChainBlock `json:"blocks"`
}

type ChainBlock struct {
	Header       BlockHeader        `json:"header"`
	Transactions []ChainTransaction `json:"transactions"`
}

type BlockHeader struct {
	Number        hexutil.Uint64 `json:"number"`
	Hash          common.Hash    `json:"hash"`
	ParentHash    common.Hash    `json:"parentHash"`
	ChangesetHash common.Hash    `json:"changesetHash"`
}

type ChainTransaction struct {
	Hash       common.Hash      `json:"hash"`
	Index      uint64           `json:"index"`
	Sender     common.Address   `json:"sender"`
	Operations []ChainOperation `json:"operations"`
}

type ChainOperation struct {
	Type          string         `json:"type"`
	OpIndex       uint64         `json:"opIndex"`
	EntityKey     common.Hash    `json:"entityKey"`
	EntityHash    common.Hash    `json:"entityHash"`
	ChangesetHash common.Hash    `json:"changesetHash"`
	Payload       hexutil.Bytes  `json:"payload"`
	ContentType   string         `json:"contentType"`
	ExpiresAt     hexutil.Uint64 `json:"expiresAt"`
	Owner         common.Address `json:"owner"`
	Attributes    []Attribute    `json:"attributes"`
}

// Attribute holds a typed attribute value.
type Attribute struct {
	ValueType string `json:"valueType"`
	Name      string `json:"name"`
	Value     string `json:"value"`
}

// CommitChainResponse is the result returned by arkiv_commitChain.
type CommitChainResponse struct {
	StateRoot common.Hash `json:"stateRoot"`
}

// RevertRequest is the input shape for arkiv_revert.
type RevertRequest struct {
	Blocks []RevertBlock `json:"blocks"`
}

// RevertBlock identifies a single block to revert by number and hash.
type RevertBlock struct {
	Number hexutil.Uint64 `json:"number"`
	Hash   common.Hash    `json:"hash"`
}

// RevertResponse is the result returned by arkiv_revert.
type RevertResponse struct {
	StateRoot common.Hash `json:"stateRoot"`
}

// ReorgRequest is the input shape for arkiv_reorg.
type ReorgRequest struct {
	RevertedBlocks []RevertBlock `json:"revertedBlocks"`
	NewBlocks      []ChainBlock  `json:"newBlocks"`
}

// ReorgResponse is the result returned by arkiv_reorg.
type ReorgResponse struct {
	StateRoot common.Hash `json:"stateRoot"`
}

// toBlockBatch converts the chain request into the internal events.BlockBatch.
func (r *CommitChainRequest) toBlockBatch() (events.BlockBatch, error) {
	blocks := make([]events.Block, 0, len(r.Blocks))
	for _, cb := range r.Blocks {
		blockNum := uint64(cb.Header.Number)
		var ops []events.Operation
		for _, tx := range cb.Transactions {
			for _, cop := range tx.Operations {
				op, err := cop.toOperation(blockNum, tx.Index, tx.Sender)
				if err != nil {
					return events.BlockBatch{}, fmt.Errorf("block 0x%x tx %s: %w", blockNum, tx.Hash, err)
				}
				ops = append(ops, op)
			}
		}
		blocks = append(blocks, events.Block{
			Number:     blockNum,
			Operations: ops,
		})
	}
	return events.BlockBatch{Blocks: blocks}, nil
}

func (cop *ChainOperation) toOperation(blockNum uint64, txIndex uint64, sender common.Address) (events.Operation, error) {
	strAttrs, numAttrs := attributesToMaps(cop.Attributes)

	op := events.Operation{
		TxIndex: txIndex,
		OpIndex: cop.OpIndex,
	}

	switch cop.Type {
	case "create":
		var btl uint64
		expiresAt := uint64(cop.ExpiresAt)
		if expiresAt > blockNum {
			btl = expiresAt - blockNum
		}
		op.Create = &events.OPCreate{
			Key:               cop.EntityKey,
			ContentType:       cop.ContentType,
			BTL:               btl,
			Owner:             cop.Owner,
			Content:           cop.Payload,
			StringAttributes:  strAttrs,
			NumericAttributes: numAttrs,
		}

	case "update":
		var btl uint64
		expiresAt := uint64(cop.ExpiresAt)
		if expiresAt > blockNum {
			btl = expiresAt - blockNum
		}
		op.Update = &events.OPUpdate{
			Key:               cop.EntityKey,
			ContentType:       cop.ContentType,
			BTL:               btl,
			Owner:             cop.Owner,
			Content:           cop.Payload,
			StringAttributes:  strAttrs,
			NumericAttributes: numAttrs,
		}

	case "delete":
		d := events.OPDelete(cop.EntityKey)
		op.Delete = &d

	case "expire":
		e := events.OPExpire(cop.EntityKey)
		op.Expire = &e

	case "extend_btl":
		var btl uint64
		expiresAt := uint64(cop.ExpiresAt)
		if expiresAt > blockNum {
			btl = expiresAt - blockNum
		}
		op.ExtendBTL = &events.OPExtendBTL{
			Key: cop.EntityKey,
			BTL: btl,
		}

	case "change_owner":
		op.ChangeOwner = &events.OPChangeOwner{
			Key:   cop.EntityKey,
			Owner: cop.Owner,
		}

	default:
		return events.Operation{}, fmt.Errorf("unknown operation type %q", cop.Type)
	}

	return op, nil
}

func attributesToMaps(attributes []Attribute) (map[string]string, map[string]uint64) {
	strAttrs := map[string]string{}
	numAttrs := map[string]uint64{}
	for _, a := range attributes {
		switch a.ValueType {
		case "string", "entityKey":
			strAttrs[a.Name] = a.Value
		case "uint":
			if v, err := hexutil.DecodeUint64(a.Value); err == nil {
				numAttrs[a.Name] = v
			}
		}
	}
	return strAttrs, numAttrs
}
