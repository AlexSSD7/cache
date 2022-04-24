package cache

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const shieldExpiry = time.Second * 5

type shieldEntry struct {
	lastAccessed time.Time
	mu           *sync.Mutex
}

type CacheEntry[T any] struct {
	Expires time.Time
	Data    T
}

// ShieldedCache is embedded in-memory shielded cache.
// "Shielded" means that duplicate requests will not be
// processed, but instead, they will wait for an existing
// request to be processed and get the cached result from it.
type ShieldedCache[T any] struct {
	objects   map[string]CacheEntry[T]
	objectsMu sync.RWMutex

	shieldsMu sync.Mutex
	shields   map[string]*shieldEntry

	gcInterval    time.Duration
	workerRunning uint32
}

// NewShieldedCache creates a new ShieldedCache instance with a customizeable
// gc interval - period between garbage collection of expired objects.
func NewShieldedCache[T any](gcInterval time.Duration) *ShieldedCache[T] {
	return &ShieldedCache[T]{
		objects: make(map[string]CacheEntry[T]),
		shields: make(map[string]*shieldEntry),

		gcInterval: gcInterval,
	}
}

// Fetch is the main read-write cache that acts as a middleware between actual fetch function
// and the application, creating a cache layer in between.
func (s *ShieldedCache[T]) Fetch(key string, ttl time.Duration, fetchFunc func() (T, error)) (*CacheEntry[T], bool, error) {
	if atomic.LoadUint32(&s.workerRunning) == 0 {
		// Ensuring that the worker is running to prevent
		// possible memory leak as GC not being active.
		return nil, false, ErrWorkerNotRunning
	}

	s.shieldsMu.Lock()
	shield := s.shields[key]
	if shield == nil {
		shield = &shieldEntry{
			lastAccessed: time.Now(),
			mu:           new(sync.Mutex),
		}
		s.shields[key] = shield
	}
	s.shieldsMu.Unlock()

	shield.mu.Lock()
	defer shield.mu.Unlock()

	s.objectsMu.RLock()
	res, ok := s.objects[key]
	s.objectsMu.RUnlock()

	if ok {
		return &res, true, nil
	}

	ret, err := fetchFunc()
	if err != nil {
		return nil, false, err
	}

	res = CacheEntry[T]{
		Expires: time.Now().Add(ttl),
		Data:    ret,
	}

	s.objectsMu.Lock()
	s.objects[key] = res
	s.objectsMu.Unlock()

	return &res, false, nil
}

// StartWorker starts the worker, the goroutine that periodically evicts
// expired objects in the cache. The GC interval is configured in ShieldedCache creation.
func (s *ShieldedCache[T]) StartWorker(ctx context.Context) error {
	if atomic.LoadUint32(&s.workerRunning) == 1 {
		return fmt.Errorf("worker already running")
	}

	atomic.StoreUint32(&s.workerRunning, 1)

	go s.runWorker(ctx)

	return nil
}

func (s *ShieldedCache[T]) runWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			s.objectsMu.Lock()
			for key := range s.objects {
				delete(s.objects, key)
			}
			s.objectsMu.Unlock()
			s.shieldsMu.Lock()
			for key := range s.shields {
				delete(s.shields, key)
			}
			s.shieldsMu.Unlock()
			atomic.StoreUint32(&s.workerRunning, 0)
			return
		case <-time.After(s.gcInterval):
			s.objectsMu.Lock()
			for key, item := range s.objects {
				if item.Expires.Before(time.Now()) {
					// Object expired
					delete(s.objects, key)
				}
			}
			s.objectsMu.Unlock()
			s.shieldsMu.Lock()
			for key, shield := range s.shields {
				if shield.lastAccessed.Add(shieldExpiry).Before(time.Now()) {
					delete(s.shields, key)
				}
			}
			s.shieldsMu.Unlock()
		}
	}
}

// Usage returns the size of underlying maps for objects, and shields.
func (s *ShieldedCache[T]) Usage() (int, int) {
	s.objectsMu.RLock()
	objectsLen := len(s.objects)
	s.objectsMu.RUnlock()

	s.shieldsMu.Lock()
	shieldsLen := len(s.shields)
	s.shieldsMu.Unlock()

	return objectsLen, shieldsLen
}

func (s *ShieldedCache[T]) DeleteObject(key string) {
	s.objectsMu.Lock()
	defer s.objectsMu.Unlock()

	delete(s.objects, key)
}
