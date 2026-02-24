package config

import (
	"log/slog"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher watches the config file and calls a callback on changes.
type Watcher struct {
	path     string
	callback func(*Config)
	watcher  *fsnotify.Watcher
	stop     chan struct{}
	once     sync.Once
}

// NewWatcher creates a new config file watcher.
func NewWatcher(path string, callback func(*Config)) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := fw.Add(path); err != nil {
		fw.Close()
		return nil, err
	}
	return &Watcher{
		path:     path,
		callback: callback,
		watcher:  fw,
		stop:     make(chan struct{}),
	}, nil
}

// Start begins watching for file changes in a goroutine.
func (w *Watcher) Start() {
	go w.run()
}

func (w *Watcher) run() {
	// Debounce: editors often do rename+create or multiple writes
	var debounce *time.Timer
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(500*time.Millisecond, func() {
					w.reload()
				})
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			slog.Error("config watcher error", "error", err)
		case <-w.stop:
			return
		}
	}
}

func (w *Watcher) reload() {
	cfg, err := Load(w.path)
	if err != nil {
		slog.Error("failed to reload config", "error", err)
		return
	}
	slog.Info("config reloaded", "path", w.path)
	w.callback(cfg)
}

// Stop stops watching for changes.
func (w *Watcher) Stop() {
	w.once.Do(func() {
		close(w.stop)
		w.watcher.Close()
	})
}
