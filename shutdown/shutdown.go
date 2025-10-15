package shutdown

import (
	"context"
	"github.com/oomamontov/grace/shutdown/task"
)

type Config struct {
	layers [][]task.Task
}

// New returns empty shutdown config.
func New() Config {
	return Config{}
}

// WithDefaultValues returns config with empty fields set to some reasonable defaults.
func (c Config) WithDefaultValues() Config {
	return c
}

func (c Config) Run(ctx context.Context) error {
	// TODO
	return nil
}
