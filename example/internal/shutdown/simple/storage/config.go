package storage

import (
	"github.com/oomamontov/grace/example/internal/shutdown/simple/storage/caching"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/storage/kv"
	"github.com/oomamontov/grace/example/internal/shutdown/simple/storage/relational"
)

type Config struct {
	Caching    caching.Config
	KV         kv.Config
	Relational relational.Config
}
