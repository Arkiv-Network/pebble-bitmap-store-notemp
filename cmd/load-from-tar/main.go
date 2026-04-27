package main

import (
	"crypto/rand"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	arkivevents "github.com/Arkiv-Network/arkiv-events"
	"github.com/Arkiv-Network/arkiv-events/events"
	"github.com/Arkiv-Network/arkiv-events/tariterator"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/urfave/cli/v2"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	var serverURL string

	app := &cli.App{
		Name:      "load-from-tar",
		Usage:     "Load blockchain events from a TAR archive into the Arkiv store via the JSON-RPC server",
		ArgsUsage: "<tar-file>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "server",
				Value:       "http://localhost:8545",
				Usage:       "JSON-RPC server URL",
				Destination: &serverURL,
				EnvVars:     []string{"ARKIV_SERVER"},
			},
		},
		Action: func(c *cli.Context) error {
			tarFileName := c.Args().First()
			if tarFileName == "" {
				return fmt.Errorf("tar file is required")
			}

			tarFile, err := os.Open(tarFileName)
			if err != nil {
				return fmt.Errorf("failed to open tar file: %w", err)
			}
			defer tarFile.Close()

			client, err := ethrpc.DialContext(c.Context, serverURL)
			if err != nil {
				return fmt.Errorf("connect to %s: %w", serverURL, err)
			}
			defer client.Close()

			ctx, cancel := signal.NotifyContext(c.Context, syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			iterator := arkivevents.BatchIterator(tariterator.IterateTar(200, tarFile))

			for batchOrErr := range iterator {
				if batchOrErr.Error != nil {
					return fmt.Errorf("reading tar: %w", batchOrErr.Error)
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}

				req, err := batchToChainRequest(batchOrErr.Batch)
				if err != nil {
					return fmt.Errorf("converting batch: %w", err)
				}

				first := batchOrErr.Batch.Blocks[0].Number
				last := batchOrErr.Batch.Blocks[len(batchOrErr.Batch.Blocks)-1].Number
				logger.Info("committing blocks", "first", first, "last", last)

				var resp struct {
					StateRoot common.Hash `json:"stateRoot"`
				}
				if err := client.CallContext(ctx, &resp, "arkiv_commitChain", req); err != nil {
					return fmt.Errorf("arkiv_commitChain blocks %d-%d: %w", first, last, err)
				}
				logger.Info("committed", "first", first, "last", last, "stateRoot", resp.StateRoot.Hex())
			}

			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

// batchToChainRequest converts an internal events.BlockBatch to the arkiv_commitChain
// wire format. Block hashes are generated randomly since the tar archive does not
// carry original chain headers.
func batchToChainRequest(batch events.BlockBatch) (map[string]any, error) {
	chainBlocks := make([]map[string]any, 0, len(batch.Blocks))

	var prevHash common.Hash // zero for the first block in each batch
	for _, block := range batch.Blocks {
		blockHash := randomHash()
		chainOps, err := convertOperations(block)
		if err != nil {
			return nil, fmt.Errorf("block %d: %w", block.Number, err)
		}

		chainBlocks = append(chainBlocks, map[string]any{
			"header": map[string]any{
				"number":     hexutil.EncodeUint64(block.Number),
				"hash":       blockHash.Hex(),
				"parentHash": prevHash.Hex(),
			},
			"operations": chainOps,
		})
		prevHash = blockHash
	}

	return map[string]any{"blocks": chainBlocks}, nil
}

func convertOperations(block events.Block) ([]map[string]any, error) {
	ops := make([]map[string]any, 0, len(block.Operations))
	for _, op := range block.Operations {
		chainOp, err := convertOperation(block.Number, op)
		if err != nil {
			return nil, err
		}
		ops = append(ops, chainOp)
	}
	return ops, nil
}

func convertOperation(blockNum uint64, op events.Operation) (map[string]any, error) {
	base := map[string]any{
		"txSeq": op.TxIndex,
		"opSeq": op.OpIndex,
	}

	switch {
	case op.Create != nil:
		expiresAt := uint64(0)
		if op.Create.BTL > 0 {
			expiresAt = blockNum + op.Create.BTL
		}
		base["type"] = "create"
		base["sender"] = op.Create.Owner.Hex()
		base["entityAddress"] = hashToAddress(op.Create.Key).Hex()
		base["payload"] = hexutil.Bytes(op.Create.Content)
		base["contentType"] = op.Create.ContentType
		base["expiresAt"] = expiresAt
		base["owner"] = op.Create.Owner.Hex()
		base["annotations"] = attrsToAnnotations(op.Create.StringAttributes, op.Create.NumericAttributes)

	case op.Update != nil:
		expiresAt := uint64(0)
		if op.Update.BTL > 0 {
			expiresAt = blockNum + op.Update.BTL
		}
		base["type"] = "update"
		base["sender"] = op.Update.Owner.Hex()
		base["entityAddress"] = hashToAddress(op.Update.Key).Hex()
		base["payload"] = hexutil.Bytes(op.Update.Content)
		base["contentType"] = op.Update.ContentType
		base["expiresAt"] = expiresAt
		base["owner"] = op.Update.Owner.Hex()
		base["annotations"] = attrsToAnnotations(op.Update.StringAttributes, op.Update.NumericAttributes)

	case op.Delete != nil:
		key := common.Hash(*op.Delete)
		base["type"] = "delete"
		base["entityAddress"] = hashToAddress(key).Hex()

	case op.Expire != nil:
		key := common.Hash(*op.Expire)
		base["type"] = "expire"
		base["entityAddress"] = hashToAddress(key).Hex()

	case op.ExtendBTL != nil:
		base["type"] = "extend_btl"
		base["entityAddress"] = hashToAddress(op.ExtendBTL.Key).Hex()
		base["expiresAt"] = blockNum + op.ExtendBTL.BTL

	case op.ChangeOwner != nil:
		base["type"] = "change_owner"
		base["entityAddress"] = hashToAddress(op.ChangeOwner.Key).Hex()
		base["owner"] = op.ChangeOwner.Owner.Hex()

	default:
		return nil, fmt.Errorf("unknown operation at txSeq=%d opSeq=%d", op.TxIndex, op.OpIndex)
	}

	return base, nil
}

// hashToAddress extracts the address from a 32-byte entity key.
// In chain.go the conversion is common.BytesToHash(address.Bytes()) which left-pads,
// so the address occupies the last 20 bytes of the hash.
func hashToAddress(h common.Hash) common.Address {
	return common.BytesToAddress(h[12:])
}

func attrsToAnnotations(strAttrs map[string]string, numAttrs map[string]uint64) []map[string]any {
	var annotations []map[string]any
	for k, v := range strAttrs {
		v := v // capture
		annotations = append(annotations, map[string]any{"key": k, "stringValue": v})
	}
	for k, v := range numAttrs {
		annotations = append(annotations, map[string]any{"key": k, "numericValue": v})
	}
	return annotations
}

func randomHash() common.Hash {
	var h common.Hash
	if _, err := rand.Read(h[:]); err != nil {
		panic(err)
	}
	return h
}
