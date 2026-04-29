package main

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Arkiv-Network/pebble-bitmap-store-notemp/pebblestore"
)

// ArkivService exposes the store over JSON-RPC under the "arkiv" namespace.
// Method names are lowercased by the go-ethereum rpc framework:
//   - Query       → arkiv_query
//   - CommitChain → arkiv_commitChain
//   - Revert      → arkiv_revert
//   - Reorg       → arkiv_reorg
type ArkivService struct {
	store *pebblestore.PebbleStore
	queue *CommitQueue
	log   *slog.Logger
}

// Query handles arkiv_query requests.
func (s *ArkivService) Query(ctx context.Context, queryStr string, options *pebblestore.Options) (*pebblestore.QueryResponse, error) {
	return s.store.QueryEntities(ctx, queryStr, options)
}

// CommitChain handles arkiv_commitChain requests. It converts the chain format
// into the internal BlockBatch, blocks until the batch is committed to PebbleDB,
// and returns the last block's hash as the stateRoot.
func (s *ArkivService) CommitChain(ctx context.Context, req CommitChainRequest) (*CommitChainResponse, error) {
	if len(req.Blocks) == 0 {
		return nil, errors.New("batch must contain at least one block")
	}
	for _, b := range req.Blocks {
		totalOps := 0
		for _, tx := range b.Transactions {
			totalOps += len(tx.Operations)
			if totalOps >= 1 {
				s.log.Info("processing block", "number", b.Header.Number, "transactions", len(b.Transactions), "operations", totalOps)
			}
		}
		s.log.Info("commitChain received block", "number", b.Header.Number, "transactions", len(b.Transactions), "operations", totalOps)
	}
	batch, err := req.toBlockBatch()
	if err != nil {
		return nil, &rpcError{code: -32602, msg: err.Error()}
	}
	
	if err := s.queue.Push(ctx, batch); err != nil {
		return nil, err
	}
	lastHash := req.Blocks[len(req.Blocks)-1].Header.Hash
	return &CommitChainResponse{StateRoot: lastHash}, nil
}

// Revert handles arkiv_revert requests.
// TODO: implement block revert in pebblestore — store-level API not yet available.
func (s *ArkivService) Revert(_ context.Context, req RevertRequest) (*RevertResponse, error) {
	if len(req.Blocks) == 0 {
		return nil, errors.New("revert request must contain at least one block")
	}
	return nil, &rpcError{code: -32000, msg: "revert: not implemented"}
}

// Reorg handles arkiv_reorg requests.
// TODO: implement block reorg in pebblestore — store-level API not yet available.
func (s *ArkivService) Reorg(_ context.Context, req ReorgRequest) (*ReorgResponse, error) {
	if len(req.RevertedBlocks) == 0 && len(req.NewBlocks) == 0 {
		return nil, errors.New("reorg request must contain reverted or new blocks")
	}
	return nil, &rpcError{code: -32000, msg: "reorg: not implemented"}
}

type rpcError struct {
	code int
	msg  string
}

func (e *rpcError) Error() string  { return e.msg }
func (e *rpcError) ErrorCode() int { return e.code }
