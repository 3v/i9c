package watcher

import (
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type EventType int

const (
	EventModified EventType = iota
	EventCreated
	EventDeleted
)

type Event struct {
	Type EventType
	Path string
}

type Watcher struct {
	dir      string
	events   chan Event
	done     chan struct{}
	fsw      *fsnotify.Watcher
	mu       sync.Mutex
	debounce map[string]*time.Timer
}

func New(dir string) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{
		dir:      dir,
		events:   make(chan Event, 100),
		done:     make(chan struct{}),
		fsw:      fsw,
		debounce: make(map[string]*time.Timer),
	}, nil
}

func (w *Watcher) Events() <-chan Event {
	return w.events
}

func (w *Watcher) Start() error {
	if err := w.fsw.Add(w.dir); err != nil {
		return err
	}

	go w.loop()
	return nil
}

func (w *Watcher) Stop() {
	close(w.done)
	w.fsw.Close()
}

func (w *Watcher) loop() {
	for {
		select {
		case <-w.done:
			return
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if !isIaCFile(event.Name) {
				continue
			}
			w.debounceEvent(event)
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			log.Printf("watcher error: %v", err)
		}
	}
}

func (w *Watcher) debounceEvent(fsEvent fsnotify.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if timer, exists := w.debounce[fsEvent.Name]; exists {
		timer.Stop()
	}

	w.debounce[fsEvent.Name] = time.AfterFunc(500*time.Millisecond, func() {
		var evtType EventType
		switch {
		case fsEvent.Has(fsnotify.Create):
			evtType = EventCreated
		case fsEvent.Has(fsnotify.Remove) || fsEvent.Has(fsnotify.Rename):
			evtType = EventDeleted
		default:
			evtType = EventModified
		}

		select {
		case w.events <- Event{Type: evtType, Path: fsEvent.Name}:
		default:
		}

		w.mu.Lock()
		delete(w.debounce, fsEvent.Name)
		w.mu.Unlock()
	})
}

func isIaCFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".tf" || ext == ".tofu" || ext == ".tfvars" || ext == ".hcl"
}
