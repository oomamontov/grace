package service

import (
	"context"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/storage/caching"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/storage/kv"
	"log/slog"
	"time"
)

type Service struct {
	kvStorage         *kv.Storage
	relationalStorage *caching.Storage
	log               *slog.Logger
}

func New(cfg Config, kvStorage *kv.Storage, rStorage *caching.Storage, log *slog.Logger) *Service {
	return &Service{
		kvStorage:         kvStorage,
		relationalStorage: rStorage,
		log:               log.With(slog.String("component", "service")),
	}
}

func (s *Service) Run(ctx context.Context) error {
	s.log.Info("Starting service workers...")
	<-ctx.Done()
	s.log.Info("Got shutdown signal, stopping service workers...")
	time.Sleep(2 * time.Second)
	s.log.Info("Done")
	return nil
}
