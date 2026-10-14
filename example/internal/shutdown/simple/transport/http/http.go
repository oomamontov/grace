package http

import (
	"context"
	"log/slog"
	"time"

	"github.com/oomamontov/grace/example/internal/shutdown/simple/service"
)

type Server struct {
	service *service.Service
	log     *slog.Logger
}

func New(cfg Config, svc *service.Service, log *slog.Logger) *Server {
	return &Server{
		service: svc,
		log:     log.With(slog.String("component", "http-server")),
	}
}

func (s *Server) Init(ctx context.Context) error {
	s.log.Info("Binding to address...")
	s.log.Info("Ready to handle requests")

	return nil
}

// Run serves requests until the context is cancelled, like http.Server.ListenAndServe
// that returns once http.Server.Shutdown is called.
func (s *Server) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			s.log.Info("Stopped accepting new requests")

			return nil
		case <-time.After(3 * time.Second):
			s.log.Info("Handling request")
		}
	}
}

// Stop waits for unfinished requests within the deadline of ctx,
// like http.Server.Shutdown.
func (s *Server) Stop(ctx context.Context) error {
	s.log.Info("Got shutdown signal, handling unfinished requests...")

	select {
	case <-time.After(3 * time.Second):
		s.log.Info("Done")

		return nil
	case <-ctx.Done():
		s.log.Warn("Deadline exceeded, dropping unfinished requests")

		return ctx.Err()
	}
}
