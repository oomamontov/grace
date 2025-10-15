package relational

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
		log: log.With(slog.String("component", "relational-storage")),
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
	s.log.Info("Running migration...")
	select {
	case <-time.After(time.Duration(1+rand.IntN(9)) * time.Second):
		s.log.Info("Migration finished")
	case <-ctx.Done():
		s.log.Warn("Context cancelled, migration aborted")
	}
	s.log.Info("Ready")
	return nil
}

func (s *Storage) Run(ctx context.Context) error {
	s.log.Info("Runner started, waiting for shutdown")
	<-ctx.Done()
	s.log.Info("Got shutdown signal, finishing transactions")
	time.Sleep(2 * time.Second)
	s.log.Info("Connection closed. Done")
	return nil
}
