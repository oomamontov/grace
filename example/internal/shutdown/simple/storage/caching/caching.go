package caching

import (
	"context"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/storage/relational"
	"log/slog"
	"math/rand/v2"
	"time"
)

type Storage struct {
	inner *relational.Storage
	log   *slog.Logger
}

func New(cfg Config, inner *relational.Storage, log *slog.Logger) *Storage {
	return &Storage{
		inner: inner,
		log:   log.With(slog.String("component", "caching-storage")),
	}
}

func (s *Storage) Init(ctx context.Context) error {
	s.log.Info("Warming cache...")
	select {
	case <-time.After(time.Duration(1+rand.IntN(4)) * time.Second):
		s.log.Info("Warming done")
	case <-ctx.Done():
		s.log.Warn("Context cancelled, warming aborted")
	}
	return nil
}

func (s *Storage) Run(ctx context.Context) error {
	s.log.Info("Runner started")
	for {
		select {
		case <-ctx.Done():
			s.log.Info("Got shutdown signal. Done")
			return nil
		case <-time.After(5 * time.Second):
			s.log.Info("Cleaning old entries...")
		}
	}
}
