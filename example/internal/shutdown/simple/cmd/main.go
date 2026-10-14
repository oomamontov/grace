package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/oomamontov/grace/example/internal/shutdown/simple/config"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/service"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/storage/caching"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/storage/kv"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/storage/relational"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/transport/grpc"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/transport/http"
	"github.com/oomamontov/grace/shutdown"
)

func main() {
	log := slog.Default()
	cfg := config.Load()

	log.Info("Config loaded")
	kvStorage := kv.New(cfg.Storage.KV, log)
	rStorage := relational.New(cfg.Storage.Relational, log)
	cache := caching.New(cfg.Storage.Caching, rStorage, log)
	svc := service.New(cfg.Service, kvStorage, cache, log)
	httpServer := http.New(cfg.Transport.HTTP, svc, log)
	grpcServer := grpc.New(cfg.Transport.GRPC, svc, log)

	builder := shutdown.New().WithDefaultValues().
		WithDefaultLayerExitTimeout(5*time.Second). // each layer gets at most 5 seconds to stop
		WithShutdownTimeout(20*time.Second).        // and the whole shutdown at most 20 seconds
		Register(kvStorage, rStorage).
		Register(cache). // cache is depending on rStorage, so rStorage should be initialized beforehand
		Register(svc).
		Register(httpServer, grpcServer) // servers are independent, so they could be initialized and stopped concurrently
	if err := builder.Run(context.Background()); err != nil {
		log.With(slog.String("error", err.Error())).Error("error running application")
	}
}
