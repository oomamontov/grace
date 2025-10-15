package config

import (
	"github.com/oomamontov/grace/example/internal/shutdown/simple/service"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/storage"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/transport"
)

type Config struct {
	Storage   storage.Config
	Service   service.Config
	Transport transport.Config
}

func Load() Config {
	return Config{}
}
