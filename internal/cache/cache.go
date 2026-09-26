package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/dgraph-io/ristretto"
)

type Cache interface {
	Get(context.Context, string) (any, bool, error)
	Set(context.Context, string, any, time.Duration) error
	Delete(context.Context, string) error
	Close()
}

type StaleValue struct {
	Value      any
	FreshUntil time.Time
}

type Ristretto struct {
	cache *ristretto.Cache
}

func NewRistretto(maxEntries int64) (*Ristretto, error) {
	if maxEntries < 1 {
		return nil, fmt.Errorf("cache max size must be positive")
	}
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: maxEntries * 10,
		MaxCost:     maxEntries,
		BufferItems: 64,
	})
	if err != nil {
		return nil, err
	}
	return &Ristretto{cache: cache}, nil
}

func (r *Ristretto) Get(ctx context.Context, key string) (any, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	value, ok := r.cache.Get(key)
	return value, ok, nil
}

func (r *Ristretto) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if ttl <= 0 {
		return fmt.Errorf("cache ttl must be positive")
	}
	if !r.cache.SetWithTTL(key, value, 1, ttl) {
		return fmt.Errorf("cache rejected entry")
	}
	r.cache.Wait()
	return nil
}

func (r *Ristretto) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.cache.Del(key)
	return nil
}

func (r *Ristretto) Close() {
	r.cache.Close()
}
