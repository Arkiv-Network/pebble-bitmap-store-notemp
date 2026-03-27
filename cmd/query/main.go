package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/Arkiv-Network/sqlite-bitmap-store-notemp/pebblestore"
	"github.com/urfave/cli/v2"
)

func main() {

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := struct {
		dbPath string
	}{}

	app := &cli.App{
		Name:  "query",
		Usage: "Query the PebbleDB database",
		Flags: []cli.Flag{
			&cli.PathFlag{
				Name:        "db-path",
				Value:       "arkiv-data.db",
				Destination: &cfg.dbPath,
				EnvVars:     []string{"DB_PATH"},
			},
		},
		Action: func(c *cli.Context) error {

			queryString := c.Args().First()

			if queryString == "" {
				return fmt.Errorf("query is required")
			}

			st, err := pebblestore.NewPebbleStore(logger, cfg.dbPath)
			if err != nil {
				return fmt.Errorf("failed to create PebbleDB store: %w", err)
			}
			defer st.Close()

			startTime := time.Now()

			r, err := st.QueryEntities(
				context.Background(),
				queryString,
				&pebblestore.Options{
					IncludeData: &pebblestore.IncludeData{
						Key:                         true,
						ContentType:                 true,
						Payload:                     true,
						Attributes:                  true,
						SyntheticAttributes:         true,
						Expiration:                  true,
						Creator:                     true,
						Owner:                       true,
						CreatedAtBlock:              true,
						LastModifiedAtBlock:         true,
						TransactionIndexInBlock:     true,
						OperationIndexInTransaction: true,
					},
				},
			)

			duration := time.Since(startTime)

			if err != nil {
				return fmt.Errorf("failed to query entities: %w", err)
			}

			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(r)

			fmt.Fprintf(os.Stderr, "Query time: %s\n", duration)

			return nil

		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
