package grpc

import (
	"context"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/service"
	"log/slog"
	"time"
)

type Server struct {
	service *service.Service
	log     *slog.Logger
}

func New(cfg Config, svc *service.Service, log *slog.Logger) *Server {
	return &Server{
		service: svc,
		log:     log.With(slog.String("component", "grpc-server")),
	}
}

func (s *Server) Init(ctx context.Context) error {
	s.log.Info("Binding to address...")
	s.log.Info("Ready to handle requests")
	return nil
}

func (s *Server) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			s.log.Info("Got shutdown signal, handling unfinished requests...")
			time.Sleep(3 * time.Second)
			s.log.Info("Done")
			return nil
		case <-time.After(2 * time.Second):
			s.log.Info("Handling request")
		}
	}
}
