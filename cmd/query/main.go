package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/urfave/cli/v2"
)

func main() {
	var (
		serverURL string
		port      int
	)

	app := &cli.App{
		Name:      "query",
		Usage:     "Query entities via the Arkiv JSON-RPC server",
		ArgsUsage: "<query>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "server",
				Value:       "http://localhost:9545",
				Usage:       "JSON-RPC server URL",
				Destination: &serverURL,
				EnvVars:     []string{"ARKIV_SERVER"},
			},
			&cli.IntFlag{
				Name:        "port",
				Value:       0,
				Usage:       "override port in the server URL (e.g. --port 8545)",
				Destination: &port,
			},
		},
		Action: func(c *cli.Context) error {
			queryString := c.Args().First()
			if queryString == "" {
				return fmt.Errorf("query is required")
			}

			if port != 0 {
				serverURL = fmt.Sprintf("http://localhost:%d", port)
			}

			client, err := ethrpc.DialContext(context.Background(), serverURL)
			if err != nil {
				return fmt.Errorf("connect to %s: %w", serverURL, err)
			}
			defer client.Close()

			options := map[string]any{
				"includeData": map[string]bool{
					"key":                         true,
					"contentType":                 true,
					"payload":                     true,
					"attributes":                  true,
					"syntheticAttributes":         true,
					"expiration":                  true,
					"creator":                     true,
					"owner":                       true,
					"createdAtBlock":              true,
					"lastModifiedAtBlock":         true,
					"transactionIndexInBlock":     true,
					"operationIndexInTransaction": true,
				},
			}

			var result json.RawMessage
			startTime := time.Now()
			if err := client.Call(&result, "arkiv_query", queryString, options); err != nil {
				return fmt.Errorf("query failed: %w", err)
			}
			duration := time.Since(startTime)

			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(result)

			fmt.Fprintf(os.Stderr, "Query time: %s\n", duration)
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
