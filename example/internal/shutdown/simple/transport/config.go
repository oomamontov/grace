package transport

import (
	"github.com/oomamontov/grace/example/internal/shutdown/simple/transport/grpc"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/transport/http"
)

type Config struct {
	HTTP http.Config
	GRPC grpc.Config
}
