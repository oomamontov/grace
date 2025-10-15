package main

import (
	"context"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/config"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/service"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/storage/caching"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/storage/kv"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/storage/relational"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/transport/grpc"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/transport/http"
	"github.com/oomamontov/grace/shutdown"
	"log/slog"
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
		Register(kvStorage, rStorage).
		Register(cache). // cache is depending on rStorage, so rStorage should be initialized beforehand
		Register(svc).
		Register(httpServer, grpcServer) // servers are independent, so they could be initialized and stopped concurrently
	if err := builder.Run(context.Background()); err != nil {
		log.With(slog.String("error", err.Error())).Error("error running application")
	}
}
