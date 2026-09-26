package cache

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/elegba-dev/elegba/internal/config"
	"github.com/redis/go-redis/v9"
)

const redisEnvelopeFormat = "elegba-cache-v1"

type redisEnvelope struct {
	Format     string          `json:"format"`
	Value      json.RawMessage `json:"value"`
	FreshUntil *time.Time      `json:"freshUntil,omitempty"`
}

type Redis struct {
	client *redis.Client
}

func NewRedis(cfg config.Cache) (*Redis, error) {
	options := &redis.Options{
		Addr:         cfg.Address,
		Username:     cfg.Username,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MaxRetries:   cfg.MaxRetries,
		DialTimeout:  time.Duration(cfg.DialTimeout),
		ReadTimeout:  time.Duration(cfg.ReadTimeout),
		WriteTimeout: time.Duration(cfg.WriteTimeout),
	}
	if cfg.TLS != nil {
		tlsConfig, err := newRedisTLSConfig(cfg.TLS)
		if err != nil {
			return nil, err
		}
		options.TLSConfig = tlsConfig
	}
	return &Redis{client: redis.NewClient(options)}, nil
}

func newRedisTLSConfig(cfg *config.CacheTLSConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         cfg.ServerName,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
	}
	if cfg.CAFile != "" {
		rootCAs, err := x509.SystemCertPool()
		if err != nil || rootCAs == nil {
			rootCAs = x509.NewCertPool()
		}
		certificate, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read Redis CA file: %w", err)
		}
		if !rootCAs.AppendCertsFromPEM(certificate) {
			return nil, fmt.Errorf("Redis CA file contains no certificates")
		}
		tlsConfig.RootCAs = rootCAs
	}
	if cfg.CertFile != "" {
		certificate, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load Redis client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	return tlsConfig, nil
}

func (r *Redis) Get(ctx context.Context, key string) (any, bool, error) {
	data, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("Redis cache read: %w", err)
	}
	var envelope redisEnvelope
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.Format == redisEnvelopeFormat {
		value, err := decodeJSONValue(envelope.Value)
		if err != nil {
			return nil, false, fmt.Errorf("decode Redis cache entry: %w", err)
		}
		if envelope.FreshUntil != nil {
			return StaleValue{Value: value, FreshUntil: *envelope.FreshUntil}, true, nil
		}
		return value, true, nil
	}
	value, err := decodeJSONValue(data)
	if err != nil {
		return nil, false, fmt.Errorf("decode Redis cache entry: %w", err)
	}
	return value, true, nil
}

func (r *Redis) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("cache ttl must be positive")
	}
	var freshUntil *time.Time
	if stale, ok := value.(StaleValue); ok {
		value = stale.Value
		freshUntil = &stale.FreshUntil
	}
	encodedValue, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode Redis cache entry: %w", err)
	}
	encoded, err := json.Marshal(redisEnvelope{Format: redisEnvelopeFormat, Value: encodedValue, FreshUntil: freshUntil})
	if err != nil {
		return fmt.Errorf("encode Redis cache envelope: %w", err)
	}
	if err := r.client.Set(ctx, key, encoded, ttl).Err(); err != nil {
		return fmt.Errorf("Redis cache write: %w", err)
	}
	return nil
}

func (r *Redis) Delete(ctx context.Context, key string) error {
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("Redis cache delete: %w", err)
	}
	return nil
}

func (r *Redis) Close() {
	_ = r.client.Close()
}

func decodeJSONValue(data []byte) (any, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}
