package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/urfave/cli/v2"
)

type config struct {
	serverURL     string
	entityAddress string
	contentType   string
	payload       string
	owner         string
	btl           uint64
	strAttrs      cli.StringSlice
	numAttrs      cli.StringSlice
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	var cfg config

	app := &cli.App{
		Name:      "create-entity",
		Usage:     "Create a new entity in the Arkiv store via the JSON-RPC server",
		ArgsUsage: "[payload]",
		Description: `Sends an arkiv_commitChain request containing a single Create operation.

Payload can be provided as the first argument or via --payload.
Fields not supplied are randomised:
  --entity-address → random 20-byte address
  --owner          → random 20-byte address`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "server",
				Value:       "http://localhost:8545",
				Usage:       "JSON-RPC server URL",
				Destination: &cfg.serverURL,
				EnvVars:     []string{"ARKIV_SERVER"},
			},
			&cli.StringFlag{
				Name:        "entity-address",
				Usage:       "Entity address as 0x-prefixed hex (20 bytes). Randomised if omitted.",
				Destination: &cfg.entityAddress,
			},
			&cli.StringFlag{
				Name:        "content-type",
				Value:       "application/octet-stream",
				Destination: &cfg.contentType,
			},
			&cli.StringFlag{
				Name:        "payload",
				Usage:       "Payload as UTF-8 text or 0x-prefixed hex. Random 32 bytes if omitted.",
				Destination: &cfg.payload,
			},
			&cli.StringFlag{
				Name:        "owner",
				Usage:       "Owner address as 0x-prefixed hex (20 bytes). Randomised if omitted.",
				Destination: &cfg.owner,
			},
			&cli.Uint64Flag{
				Name:        "btl",
				Usage:       "Blocks-to-live added to current block number to compute expiresAt (0 = no expiry)",
				Value:       0,
				Destination: &cfg.btl,
			},
			&cli.StringSliceFlag{
				Name:        "attr",
				Aliases:     []string{"a"},
				Usage:       "String annotation as key=value. Repeatable.",
				Destination: &cfg.strAttrs,
			},
			&cli.StringSliceFlag{
				Name:        "num-attr",
				Aliases:     []string{"n"},
				Usage:       "Numeric annotation as key=uint64. Repeatable.",
				Destination: &cfg.numAttrs,
			},
		},
		Action: func(c *cli.Context) error {
			return run(c, logger, &cfg)
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func run(c *cli.Context, logger *slog.Logger, cfg *config) error {
	// --- Resolve entity address ---
	var entityAddress common.Address
	if cfg.entityAddress != "" {
		entityAddress = common.HexToAddress(cfg.entityAddress)
	} else {
		entityAddress = randomAddress()
		logger.Info("generated random entity address", "address", entityAddress.Hex())
	}

	// --- Resolve owner ---
	var owner common.Address
	if cfg.owner != "" {
		owner = common.HexToAddress(cfg.owner)
	} else {
		owner = randomAddress()
		logger.Info("generated random owner", "owner", owner.Hex())
	}

	// --- Resolve payload ---
	var content hexutil.Bytes
	raw := cfg.payload
	if raw == "" && c.Args().Len() > 0 {
		raw = c.Args().First()
	}
	if raw == "" {
		content = randomBytes(32)
		logger.Info("generated random payload", "bytes", len(content))
	} else if strings.HasPrefix(raw, "0x") || strings.HasPrefix(raw, "0X") {
		b, err := hex.DecodeString(raw[2:])
		if err != nil {
			return fmt.Errorf("invalid hex payload: %w", err)
		}
		content = b
	} else {
		content = []byte(raw)
	}

	// --- Build annotations ---
	var annotations []map[string]any
	for _, kv := range cfg.strAttrs.Value() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return fmt.Errorf("invalid --attr %q: expected key=value", kv)
		}
		annotations = append(annotations, map[string]any{"key": k, "stringValue": v})
	}
	for _, kv := range cfg.numAttrs.Value() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return fmt.Errorf("invalid --num-attr %q: expected key=uint64", kv)
		}
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid --num-attr %q: %w", kv, err)
		}
		annotations = append(annotations, map[string]any{"key": k, "numericValue": n})
	}

	// --- Connect and get current block number ---
	client, err := ethrpc.DialContext(context.Background(), cfg.serverURL)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", cfg.serverURL, err)
	}
	defer client.Close()

	var queryResp struct {
		BlockNumber hexutil.Uint64 `json:"blockNumber"`
	}
	oneResult := hexutil.Uint64(1)
	if err := client.Call(&queryResp, "arkiv_query", "*", map[string]any{
		"resultsPerPage": &oneResult,
	}); err != nil {
		return fmt.Errorf("fetch current block number: %w", err)
	}
	blockNumber := uint64(queryResp.BlockNumber) + 1
	expiresAt := uint64(0)
	if cfg.btl > 0 {
		expiresAt = blockNumber + cfg.btl
	}
	logger.Info("targeting block", "block", blockNumber)

	// --- Build arkiv_commitChain request ---
	blockHash := randomHash()
	req := map[string]any{
		"blocks": []map[string]any{
			{
				"header": map[string]any{
					"number":     hexutil.EncodeUint64(blockNumber),
					"hash":       blockHash.Hex(),
					"parentHash": common.Hash{}.Hex(),
				},
				"operations": []map[string]any{
					{
						"type":          "create",
						"txSeq":         0,
						"opSeq":         0,
						"sender":        owner.Hex(),
						"entityAddress": entityAddress.Hex(),
						"payload":       content,
						"contentType":   cfg.contentType,
						"expiresAt":     expiresAt,
						"owner":         owner.Hex(),
						"annotations":   annotations,
					},
				},
			},
		},
	}

	var resp struct {
		StateRoot common.Hash `json:"stateRoot"`
	}
	if err := client.Call(&resp, "arkiv_commitChain", req); err != nil {
		return fmt.Errorf("arkiv_commitChain: %w", err)
	}

	// --- Print result ---
	result := map[string]any{
		"stateRoot":     resp.StateRoot.Hex(),
		"block":         blockNumber,
		"entityAddress": entityAddress.Hex(),
		"owner":         owner.Hex(),
		"contentType":   cfg.contentType,
		"payloadSize":   len(content),
		"expiresAt":     expiresAt,
	}
	if len(annotations) > 0 {
		result["annotations"] = annotations
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func randomHash() common.Hash {
	var h common.Hash
	if _, err := rand.Read(h[:]); err != nil {
		panic(err)
	}
	return h
}

func randomAddress() common.Address {
	var a common.Address
	if _, err := rand.Read(a[:]); err != nil {
		panic(err)
	}
	return a
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}
