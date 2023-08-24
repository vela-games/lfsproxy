package cache

import (
	"context"
	"time"

	"github.com/allegro/bigcache/v3"
)

type Cache interface {
	Get(key string) ([]byte, error)
	Set(key string, entry []byte) error
	Delete(key string) error
}

func NewCache(ctx context.Context, cacheEviction time.Duration) (Cache, error) {
	cache, err := bigcache.New(ctx, bigcache.DefaultConfig(cacheEviction))
	if err != nil {
		return nil, err
	}

	return cache, nil
}
