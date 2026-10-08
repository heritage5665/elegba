package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elegba-dev/elegba/internal/config"
	"github.com/fsnotify/fsnotify"
)

const defaultHotReloadDebounce = 500 * time.Millisecond

type Server struct {
	engine atomic.Pointer[Engine]
	config *config.Config
	mu     sync.Mutex

	watchCancel context.CancelFunc
	watchDone   chan struct{}
}

func NewServer(cfg *config.Config) (*Server, error) {
	if cfg == nil {
		return nil, errors.New("configuration is required")
	}
	appEngine, err := New(cfg)
	if err != nil {
		return nil, err
	}
	server := &Server{config: cfg}
	server.engine.Store(appEngine)
	return server, nil
}

func (s *Server) Engine() *Engine {
	return s.engine.Load()
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.engine.Load().ServeHTTP(w, r)
}

func (s *Server) MetricsHandler() http.Handler {
	return s.Engine().MetricsHandler()
}

func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
	s.Engine().HealthHandler(w, r)
}

func (s *Server) ReadyHandler(w http.ResponseWriter, r *http.Request) {
	s.Engine().ReadyHandler(w, r)
}

func (s *Server) ReloadFile(path string) error {
	newConfig, err := config.LoadConfig(path)
	if err != nil {
		return fmt.Errorf("load replacement configuration: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if !reflect.DeepEqual(s.config.Server, newConfig.Server) || !reflect.DeepEqual(s.config.Caches, newConfig.Caches) {
		return errors.New("hot reload does not support changes to server or caches; restart Elegba")
	}

	newEngine, err := New(newConfig)
	if err != nil {
		return fmt.Errorf("build replacement engine: %w", err)
	}
	oldEngine := s.engine.Swap(newEngine)
	if oldEngine == nil {
		return errors.New("running engine is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(s.config.Server.ShutdownTimeout))
	defer cancel()
	oldEngine.Drain(ctx)
	s.config = newConfig
	slog.Info("configuration reloaded", "path", path)
	return nil
}

func (s *Server) Watch(ctx context.Context, path string, debounce time.Duration) error {
	if debounce <= 0 {
		debounce = defaultHotReloadDebounce
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	watchDir := filepath.Dir(path)
	if err := watcher.Add(watchDir); err != nil {
		_ = watcher.Close()
		return err
	}

	watchCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	if s.watchCancel != nil {
		s.mu.Unlock()
		_ = watcher.Close()
		return errors.New("configuration watcher already running")
	}
	s.watchCancel = cancel
	s.watchDone = make(chan struct{})
	s.mu.Unlock()

	go func() {
		defer close(s.watchDone)
		defer watcher.Close()
		var timer *time.Timer
		var timerC <-chan time.Time
		for {
			select {
			case <-watchCtx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if filepath.Base(event.Name) != filepath.Base(path) {
					continue
				}
				if timer == nil {
					timer = time.NewTimer(debounce)
					timerC = timer.C
				} else {
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					timer.Reset(debounce)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				slog.Warn("configuration watcher error", "error", err)
			case <-timerC:
				if err := s.ReloadFile(path); err != nil {
					slog.Error("configuration reload failed", "path", path, "error", err)
				}
				timer = nil
				timerC = nil
			}
		}
	}()
	return nil
}

func (s *Server) Close() {
	s.mu.Lock()
	cancel := s.watchCancel
	s.watchCancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if appEngine := s.engine.Load(); appEngine != nil {
		appEngine.Close()
	}
}
