package speicher

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type (
	// memoryMap is a Map implementation that keeps all elements in memory.
	memoryMap[T any] struct {
		data     map[string]T
		location string
		mut      sync.RWMutex

		timerMut     sync.Mutex
		saveTimer    *time.Timer
		maxSaveTimer *time.Timer
		saveOnce     *sync.Once
	}

	// Map is a thread-safe key-value data store interface that provides basic
	// CRUD operations, predicate-based search, and iteration functionality.
	Map[T any] interface {
		// Get retrieves an element associated with the given key.
		// It returns the value and a boolean indicating whether the key exists.
		Get(key string) (T, bool)

		// Find searches for an element that satisfies the given predicate.
		// It returns the found value and a boolean indicating if a match was found.
		Find(func(T) bool) (value T, found bool)

		// FindAll retrieves all elements that satisfy the given predicate.
		// It returns a slice containing all matching elements.
		FindAll(func(T) bool) (values []T)

		// Has checks if an element with the given key exists in the data store.
		// It returns true if the key exists.
		Has(key string) bool

		// Set adds or updates the element associated with the given key.
		// If the key already exists, its value is overwritten.
		Set(key string, value T)

		// Overwrite replaces the entire data store with the provided map.
		Overwrite(map[string]T)

		// RangeKV returns a read-only channel that emits key-value pair elements
		// (as MapRangeEl) from the data store, along with a cancellation function
		// to terminate the iteration when desired.
		RangeKV() (<-chan MapRangeEl[T], func())

		// RangeV returns a read-only channel that emits only the values stored in the
		// data store, along with a cancellation function to terminate the iteration.
		RangeV() (<-chan T, func())

		// Iterate iterates over the Map and calls the provided function for each element.
		Iterate(yield func(key string, value T) bool)

		// Save persists the current state of the data store.
		// It returns an error if the save operation fails.
		Save() error

		// Lock acquires the write lock for the data store to allow safe updates.
		// Don't forget to use Unlock when you are done.
		Lock()

		// Unlock releases the write lock for the data store.
		Unlock()

		// RLock acquires the read lock for the data store to allow safe reading.
		// Don't forget to use RUnlock when you are done.
		RLock()

		// RUnlock releases the read lock for the data store.
		RUnlock()
	}

	// MapRangeEl represents a key-value pair element emitted by the Map's RangeKV method.
	MapRangeEl[T any] struct {
		Key   string
		Value T
	}
)

func (m *memoryMap[T]) RangeKV() (<-chan MapRangeEl[T], func()) {
	ch := make(chan MapRangeEl[T])
	done := make(chan struct{})
	cancel := func() {
		select {
		case <-done:
			// channel already closed, no-op
			return
		default:
			if done != nil {
				close(done)
				done = nil
			}
		}
	}

	go func() {
		defer func() {
			if done != nil {
				close(done)
				done = nil
			}
			if ch != nil {
				close(ch)
				ch = nil
			}
		}()
		for key, value := range m.data {
			select {
			case <-done:
				return
			case ch <- MapRangeEl[T]{Key: key, Value: value}:
			}
		}
	}()

	return ch, cancel
}

func (m *memoryMap[T]) RangeV() (<-chan T, func()) {
	ch := make(chan T)
	done := make(chan struct{})
	cancel := func() {
		select {
		case <-done:
			// channel already closed, no-op
			return
		default:
			close(done)
		}
	}

	go func() {
		defer close(ch)
		defer close(done)
		for _, value := range m.data {
			select {
			case <-done:
				return
			case ch <- value:
			}
		}
	}()

	return ch, cancel
}

func (m *memoryMap[T]) Iterate(yield func(key string, value T) bool) {
	m.RLock()
	defer m.RUnlock()
	for key, value := range m.data {
		if !yield(key, value) {
			break
		}
	}
}

func (m *memoryMap[T]) Get(key string) (value T, found bool) {
	value, found = m.data[key]
	return
}

func (m *memoryMap[T]) Find(f func(T) bool) (value T, found bool) {
	for _, value = range m.data {
		if f(value) {
			found = true
			return
		}
	}
	found = false
	return
}

func (m *memoryMap[T]) FindAll(f func(T) bool) (values []T) {
	for _, value := range m.data {
		if f(value) {
			values = append(values, value)
		}
	}
	return
}

func (m *memoryMap[T]) Has(key string) bool {
	_, ok := m.data[key]
	return ok
}

func (m *memoryMap[T]) Set(key string, value T) {
	m.data[key] = value
}

func (m *memoryMap[T]) Overwrite(values map[string]T) {
	m.data = values
}

func (m *memoryMap[T]) Lock() {
	m.mut.Lock()
}
func (m *memoryMap[T]) Unlock() {
	m.mut.Unlock()
	notifyChanged(m)
}

func (m *memoryMap[T]) RLock() {
	m.mut.RLock()
}
func (m *memoryMap[T]) RUnlock() {
	m.mut.RUnlock()
}

func (m *memoryMap[T]) Save() error {
	m.RLock()
	defer m.RUnlock()

	f, err := os.Create(m.location)
	if err != nil {
		return errors.Join(fmt.Errorf("failed to open file '%s'", m.location), err)
	}
	encoder := json.NewEncoder(f)
	if err := encoder.Encode(m.data); err != nil {
		return errors.Join(fmt.Errorf("failed to encode json file '%s'", m.location), err)
	}
	return nil
}

func LoadMap[T any](location string) (Map[T], error) {
	if strings.HasSuffix(location, ".json") {
		if m, err := loadMapFromJsonFile[T](location); err != nil {
			return nil, errors.Join(fmt.Errorf("unable to load map from file '%s'", location), err)
		} else {
			return m, nil
		}
	}
	return nil, fmt.Errorf("unable to find loader for '%s'", location)
}

func loadMapFromJsonFile[T any](location string) (Map[T], error) {
	m := &memoryMap[T]{data: map[string]T{}, location: location}
	f, err := os.Open(location)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(filepath.Dir(location), 0740)
			return m, err
		}
		return nil, errors.Join(fmt.Errorf("failed to open file '%s' (but exists)", location), err)
	}
	decoder := json.NewDecoder(f)
	if err := decoder.Decode(&m.data); err != nil {
		return nil, errors.Join(fmt.Errorf("failed to decode json file '%s'", location), err)
	}
	return m, nil
}

func (m *memoryMap[T]) getSaveTimer() *time.Timer {
	m.timerMut.Lock()
	defer m.timerMut.Unlock()
	return m.saveTimer
}

func (m *memoryMap[T]) setSaveTimer(t *time.Timer) {
	m.timerMut.Lock()
	defer m.timerMut.Unlock()
	m.saveTimer = t
}

func (m *memoryMap[T]) getMaxSaveTimer() *time.Timer {
	m.timerMut.Lock()
	defer m.timerMut.Unlock()
	return m.maxSaveTimer
}

func (m *memoryMap[T]) setMaxSaveTimer(t *time.Timer) {
	m.timerMut.Lock()
	defer m.timerMut.Unlock()
	m.maxSaveTimer = t
}

func (m *memoryMap[T]) getSaveOnce() *sync.Once {
	m.timerMut.Lock()
	defer m.timerMut.Unlock()
	return m.saveOnce
}

func (m *memoryMap[T]) setSaveOnce(o *sync.Once) {
	m.timerMut.Lock()
	defer m.timerMut.Unlock()
	m.saveOnce = o
}

func (m *memoryMap[T]) WriteE(f func(m *memoryMap[T]) (any, error)) (any, error) {
	m.Lock()
	defer m.Unlock()
	return f(m)
}

func (m *memoryMap[T]) Write(f func(m *memoryMap[T]) any) any {
	m.Lock()
	defer m.Unlock()
	return f(m)
}

func (m *memoryMap[T]) ReadE(f func(m *memoryMap[T]) (any, error)) (any, error) {
	m.RLock()
	defer m.RUnlock()
	return f(m)
}

func (m *memoryMap[T]) Read(f func(m *memoryMap[T]) any) any {
	m.RLock()
	defer m.RUnlock()
	return f(m)
}
