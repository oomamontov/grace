package kv

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"
)

type Storage struct {
	log *slog.Logger
}

func New(cfg Config, log *slog.Logger) *Storage {
	return &Storage{
		log: log.With(slog.String("component", "kv-storage")),
	}
}

func (s *Storage) Init(ctx context.Context) error {
	s.log.Info("Connecting to db...")
	select {
	case <-time.After(time.Duration(1+rand.IntN(4)) * time.Second):
		s.log.Info("Connected")
	case <-ctx.Done():
		s.log.Warn("Context cancelled, connection aborted")
	}
	return nil
}

func (s *Storage) Run(ctx context.Context) error {
	s.log.Info("Runner started, waiting for shutdown")
	<-ctx.Done()
	s.log.Info("Got shutdown signal, closing connection")
	s.log.Info("Done")
	return nil
}
