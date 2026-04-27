package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Arkiv-Network/pebble-bitmap-store-notemp/pebblestore"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
)

const (
	httpBodyLimit  = 64 << 20 // 64 MiB — large block batches can exceed the 5 MiB default
	shutdownTimeout = 30 * time.Second
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := struct {
		dbPath     string
		listenAddr string
	}{}

	app := &cli.App{
		Name:  "server",
		Usage: "Expose the Arkiv bitmap store over JSON-RPC",
		Flags: []cli.Flag{
			&cli.PathFlag{
				Name:        "db-path",
				Value:       "arkiv-data.db",
				Destination: &cfg.dbPath,
				EnvVars:     []string{"DB_PATH"},
			},
			&cli.StringFlag{
				Name:        "listen",
				Value:       ":9545",
				Destination: &cfg.listenAddr,
				EnvVars:     []string{"LISTEN_ADDR"},
			},
		},
		Action: func(c *cli.Context) error {
			return run(c.Context, logger, cfg.dbPath, cfg.listenAddr)
		},
	}

	// Use a signal-aware root context so the CLI framework can propagate cancellation.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := app.RunContext(ctx, os.Args); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, logger *slog.Logger, dbPath, listenAddr string) error {
	store, err := pebblestore.NewPebbleStore(logger, dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}

	queue := NewCommitQueue()

	// errgroup propagates FollowEvents errors and cancels the context on failure.
	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		err := store.FollowEvents(gCtx, queue.Iterator())
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("follow events: %w", err)
		}
		return nil
	})

	rpcServer := rpc.NewServer()
	rpcServer.SetHTTPBodyLimit(httpBodyLimit)
	if err := rpcServer.RegisterName("arkiv", &ArkivService{store: store, queue: queue, log: logger}); err != nil {
		return fmt.Errorf("register rpc service: %w", err)
	}

	httpServer := &http.Server{
		Addr:         listenAddr,
		Handler:      rpcServer,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
	}

	g.Go(func() error {
		logger.Info("listening", "addr", listenAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	})

	// Block until the root context is cancelled (signal) or a goroutine fails.
	<-gCtx.Done()

	logger.Info("shutting down")

	// Drain in-flight HTTP requests before closing the commit queue.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown error", "err", err)
	}
	rpcServer.Stop()
	queue.Close()

	if err := g.Wait(); err != nil {
		logger.Error("server exited with error", "err", err)
	}

	if err := store.Close(); err != nil {
		logger.Error("store close error", "err", err)
	}

	return nil
}
