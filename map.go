package speicher

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bloodmagesoftware/speicher/v2/clone"
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
		clone        func(T) T
	}

	memoryMapTx[T any] struct {
		m          *memoryMap[T]
		mut        sync.Mutex
		changes    map[string]*T
		committed  bool
		rolledBack bool
	}

	MapIterator[T any] func(yield func(key string, value T) bool)
	KVPair[T any]      struct {
		key   string
		value T
	}

	MapOrderBy[T any] func(a KVPair[T], b KVPair[T]) bool

	// MapTx is a read-write transaction for the Map data store. Reading from it will return the changes made in the transaction.
	MapTx[T any] interface {
		// Get is same as Map.Get
		Get(key string) (*T, bool)
		// Find is same as Map.Find
		Find(func(T) bool) (value *T, found bool)
		// Has is same as Map.Has
		Has(key string) bool
		// Set is same as Map.Set
		Set(key string, value T)
		// Delete is same as Map.Delete
		Delete(key string)
		// Select is same as Map.Select
		Select() MapIterator[*T]

		// Commit commits the changes made in the transaction. No-op if the transaction is already committed or rolled back.
		Commit()
		// Rollback rolls back the changes made in the transaction and makes Commit a no-op.
		Rollback()
	}

	// Map is a thread-safe key-value data store interface that provides basic
	// CRUD operations, predicate-based search, and iteration functionality.
	Map[T any] interface {
		// Begin starts a new transaction. Call Commit to write the changes to the data store. Call Rollback to discard the changes.
		Begin() MapTx[T]

		// WriteE executes the given function while holding exclusive write lock.
		WriteE(f func(m *memoryMap[T]) (any, error)) (any, error)

		// Write executes the given function while holding exclusive write lock.
		Write(f func(m *memoryMap[T]) any) any

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

		// Delete removes the element associated with the given key.
		Delete(key string)

		// Overwrite replaces the entire data store with the provided map.
		Overwrite(map[string]T)

		// Select iterates over the Map, filters the elements, and calls the provided function for each element until the function returns false.
		Select() MapIterator[T]

		// Save persists the current state of the data store.
		// It returns an error if the save operation fails.
		Save() error
	}

	MapSelector[T any] func(key string, value T) bool
)

func (m *memoryMap[T]) Select() MapIterator[T] {
	return func(yield func(key string, value T) bool) {
		for key, value := range m.data {
			if !yield(key, value) {
				break
			}
		}
	}
}

func (m *memoryMap[T]) Get(key string) (value T, found bool) {
	m.mut.RLock()
	defer m.mut.RUnlock()
	value, found = m.data[key]
	return
}

func (m *memoryMap[T]) Find(f func(T) bool) (value T, found bool) {
	m.mut.RLock()
	defer m.mut.RUnlock()
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
	m.mut.RLock()
	defer m.mut.RUnlock()
	for _, value := range m.data {
		if f(value) {
			values = append(values, value)
		}
	}
	return
}

func (m *memoryMap[T]) Has(key string) bool {
	m.mut.RLock()
	defer m.mut.RUnlock()
	_, ok := m.data[key]
	return ok
}

func (m *memoryMap[T]) Set(key string, value T) {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.data[key] = value
}

func (m *memoryMap[T]) Delete(key string) {
	m.mut.Lock()
	defer m.mut.Unlock()
	delete(m.data, key)
}

func (m *memoryMap[T]) Overwrite(values map[string]T) {
	m.data = values
}

func (m *memoryMap[T]) lock() {
	m.mut.Lock()
}
func (m *memoryMap[T]) unlock() {
	m.mut.Unlock()
	notifyChanged(m)
}

func (m *memoryMap[T]) rLock() {
	m.mut.RLock()
}
func (m *memoryMap[T]) rUnlock() {
	m.mut.RUnlock()
}

func (m *memoryMap[T]) Save() error {
	m.rLock()
	defer m.rUnlock()

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
	m := &memoryMap[T]{
		data:     map[string]T{},
		location: location,
		clone:    clone.CopyConstructor[T](),
	}
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
	m.lock()
	defer m.unlock()
	return f(m)
}

func (m *memoryMap[T]) Write(f func(m *memoryMap[T]) any) any {
	m.lock()
	defer m.unlock()
	return f(m)
}

func (m *memoryMap[T]) Begin() MapTx[T] {
	return &memoryMapTx[T]{m, sync.Mutex{}, map[string]*T{}, false, false}
}

func (tx *memoryMapTx[T]) Get(key string) (value *T, found bool) {
	tx.mut.Lock()
	defer tx.mut.Unlock()
	if v, ok := tx.changes[key]; ok {
		value = v
		found = true
	} else if v, ok := tx.m.data[key]; ok {
		newValue := v
		tx.changes[key] = &newValue
		value = &newValue
		found = true
	} else {
		found = false
	}
	return
}

func (tx *memoryMapTx[T]) Find(f func(T) bool) (value *T, found bool) {
	tx.mut.Lock()
	defer tx.mut.Unlock()

	for _, v := range tx.changes {
		if f(*v) {
			value = v
			found = true
			return
		}
	}
	for key, v := range tx.m.data {
		if f(v) {
			// copy the value
			newValue := v
			tx.changes[key] = &newValue
			value = &newValue
			found = true
			return
		}
	}
	found = false
	return
}

func (tx *memoryMapTx[T]) Has(key string) bool {
	tx.mut.Lock()
	defer tx.mut.Unlock()
	_, ok := tx.changes[key]
	_, ok2 := tx.m.data[key]
	return ok || ok2
}

func (tx *memoryMapTx[T]) Set(key string, value T) {
	tx.mut.Lock()
	defer tx.mut.Unlock()
	tx.changes[key] = &value
}

func (tx *memoryMapTx[T]) Delete(key string) {
	tx.mut.Lock()
	defer tx.mut.Unlock()
	tx.changes[key] = nil
}

func (tx *memoryMapTx[T]) Select() MapIterator[*T] {
	return func(yield func(key string, value *T) bool) {
		tx.mut.Lock()
		defer tx.mut.Unlock()
		for key, value := range tx.changes {
			if !yield(key, value) {
				break
			}
		}
		for key, value := range tx.m.data {
			if _, changed := tx.changes[key]; changed {
				continue
			}
			if !yield(key, &value) {
				break
			}
		}
	}
}

func (tx *memoryMapTx[T]) Commit() {
	tx.mut.Lock()
	defer tx.mut.Unlock()
	if tx.committed || tx.rolledBack {
		return
	}
	tx.committed = true
	tx.m.mut.Lock()
	defer tx.m.mut.Unlock()
	for key, value := range tx.changes {
		if value == nil {
			delete(tx.m.data, key)
		} else {
			tx.m.data[key] = *value
		}
	}
	tx.changes = nil
}

func (tx *memoryMapTx[T]) Rollback() {
	tx.mut.Lock()
	defer tx.mut.Unlock()
	tx.rolledBack = true
	tx.changes = nil
}

func (mi MapIterator[T]) Where(where MapSelector[T]) MapIterator[T] {
	return func(yield func(key string, value T) bool) {
		for key, value := range mi {
			if !where(key, value) {
				continue
			}
			if !yield(key, value) {
				break
			}
		}
	}
}

func (mi MapIterator[T]) OrderBy(orderBy MapOrderBy[T]) MapIterator[T] {
	return func(yield func(key string, value T) bool) {
		var kvPairs []KVPair[T]
		for key, value := range mi {
			kvPairs = append(kvPairs, KVPair[T]{key: key, value: value})
		}
		sort.Slice(kvPairs, func(i, j int) bool {
			return orderBy(kvPairs[i], kvPairs[j])
		})
		for _, kv := range kvPairs {
			if !yield(kv.key, kv.value) {
				break
			}
		}
	}
}

func (mi MapIterator[T]) Limit(limit int) MapIterator[T] {
	return func(yield func(key string, value T) bool) {
		i := 0
		for key, value := range mi {
			if i >= limit {
				break
			}
			if !yield(key, value) {
				break
			}
			i++
		}
	}
}
