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
	// memoryList is a List implementation that keeps all elements in memory.
	memoryList[T any] struct {
		id       storeID
		data     []T
		location string
		mut      sync.RWMutex

		timerMut     sync.Mutex
		saveTimer    *time.Timer
		maxSaveTimer *time.Timer
		saveOnce     *sync.Once
	}

	// List is a thread-safe list data store interface that provides basic
	// CRUD operations, predicate-based search, and iteration functionality.
	//
	// All operations require appropriate locking via a State object:
	//
	//	s := speicher.NewState()
	//	s.Lock(myList)
	//	defer s.Unlock(myList)
	//	myList.Append(value)
	List[T any] interface {
		lockable

		// Get returns the value at a given index of the List and a bool that indicates whether the index exists or not.
		// If no element is found, the bool result will be false.
		// Requires at least a read lock.
		Get(index int) (T, bool)

		// Find traverses the List and returns the first element that satisfies the provided predicate function.
		// If no element is found, the bool result will be false.
		// Requires at least a read lock.
		Find(func(T) bool) (value T, found bool)

		// FindAll returns all elements in the List that satisfy the provided predicate function.
		// If no elements match, it returns an empty slice.
		// Requires at least a read lock.
		FindAll(func(T) bool) (values []T)

		// Append adds the provided value to the end of the List.
		// Requires a write lock.
		Append(value T)

		// AppendUnique adds the provided value to the List only if no existing element is equal to it,
		// based on the supplied equality function. It returns true if the value was added,
		// and false otherwise.
		// Requires a write lock.
		AppendUnique(value T, equal func(a, b T) bool) bool

		// Set assigns the provided value to the element at the specified index.
		// If the index is out of bounds, it returns an error.
		// Requires a write lock.
		Set(index int, value T) error

		// Overwrite replaces the entire List with the data provided in the slice.
		// Requires a write lock.
		Overwrite([]T)

		// Len returns the number of elements currently in the List.
		// Requires at least a read lock.
		Len() int

		// Range returns a read-only channel through which the elements of the List can be iterated.
		// It also returns a cancel function to stop the iteration process if needed.
		// Requires at least a read lock.
		//
		// Deprecated: use Iterate if you need to iterate over the entire data store.
		Range() (<-chan T, func())

		// Iterate iterates over the List and calls the provided function for each element.
		// Requires at least a read lock.
		Iterate(yield func(v T) bool)

		// Save persists the current state of the List to its underlying data store.
		// It returns an error if the operation fails.
		// This method acquires its own read lock internally.
		Save() error
	}
)

func (l *memoryList[T]) Get(index int) (value T, found bool) {
	if index >= 0 && index < len(l.data) {
		value = l.data[index]
		found = true
	} else {
		found = false
	}
	return
}

func (l *memoryList[T]) Append(value T) {
	l.data = append(l.data, value)
}

func (l *memoryList[T]) AppendUnique(value T, equal func(a, b T) bool) bool {
	for _, x := range l.data {
		if equal(x, value) {
			return false
		}
	}
	l.data = append(l.data, value)
	return true
}

func (l *memoryList[T]) Find(f func(T) bool) (value T, found bool) {
	for _, value = range l.data {
		if f(value) {
			found = true
			return
		}
	}
	found = false
	return
}

func (l *memoryList[T]) FindAll(f func(T) bool) (values []T) {
	for _, value := range l.data {
		if f(value) {
			values = append(values, value)
		}
	}
	return
}

func (l *memoryList[T]) Set(index int, value T) error {
	if index < 0 || index >= len(l.data) {
		return fmt.Errorf("index out of range")
	}
	l.data[index] = value
	return nil
}

func (l *memoryList[T]) Overwrite(values []T) {
	l.data = values
}

func (l *memoryList[T]) Len() int {
	return len(l.data)
}

func (l *memoryList[T]) Range() (<-chan T, func()) {
	// Copy data to a slice to avoid data race with goroutine
	// The caller is expected to hold a read lock during this call
	values := make([]T, len(l.data))
	copy(values, l.data)

	ch := make(chan T)
	done := make(chan struct{})
	var closeOnce sync.Once
	cancel := func() {
		closeOnce.Do(func() {
			close(done)
		})
	}

	go func() {
		defer close(ch)
		for _, value := range values {
			select {
			case <-done:
				return
			case ch <- value:
			}
		}
	}()

	return ch, cancel
}

func (l *memoryList[T]) Iterate(yield func(v T) bool) {
	for _, value := range l.data {
		if !yield(value) {
			break
		}
	}
}

func (l *memoryList[T]) getStoreID() storeID {
	return l.id
}

func (l *memoryList[T]) getMutex() *sync.RWMutex {
	return &l.mut
}

func (l *memoryList[T]) Save() error {
	s := NewState()
	s.RLock(l)
	defer s.RUnlock(l)

	f, err := os.Create(l.location)
	if err != nil {
		return errors.Join(fmt.Errorf("failed to open file '%s'", l.location), err)
	}
	defer f.Close()
	encoder := json.NewEncoder(f)
	if err := encoder.Encode(l.data); err != nil {
		return errors.Join(fmt.Errorf("failed to encode json file '%s'", l.location), err)
	}
	return nil
}

func LoadList[T any](location string) (List[T], error) {
	if strings.HasSuffix(location, ".json") {
		if l, err := loadListFromJsonFile[T](location); err != nil {
			return nil, errors.Join(fmt.Errorf("unable to load list from file '%s'", location), err)
		} else {
			return l, nil
		}
	}
	return nil, fmt.Errorf("unable to find loader for '%s'", location)
}

func loadListFromJsonFile[T any](location string) (List[T], error) {
	l := &memoryList[T]{
		id:       newStoreID(),
		location: location,
		data:     make([]T, 0),
	}
	f, err := os.Open(location)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(filepath.Dir(location), 0740)
			return l, err
		}
		return nil, errors.Join(fmt.Errorf("failed to open file '%s' (file exists)", location), err)
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	if err := decoder.Decode(&l.data); err != nil {
		return nil, errors.Join(fmt.Errorf("failed to decode json file '%s'", location), err)
	}
	return l, nil
}

func (l *memoryList[T]) getSaveTimer() *time.Timer {
	l.timerMut.Lock()
	defer l.timerMut.Unlock()
	return l.saveTimer
}

func (l *memoryList[T]) setSaveTimer(t *time.Timer) {
	l.timerMut.Lock()
	defer l.timerMut.Unlock()
	l.saveTimer = t
}

func (l *memoryList[T]) getMaxSaveTimer() *time.Timer {
	l.timerMut.Lock()
	defer l.timerMut.Unlock()
	return l.maxSaveTimer
}

func (l *memoryList[T]) setMaxSaveTimer(t *time.Timer) {
	l.timerMut.Lock()
	defer l.timerMut.Unlock()
	l.maxSaveTimer = t
}

func (l *memoryList[T]) getSaveOnce() *sync.Once {
	l.timerMut.Lock()
	defer l.timerMut.Unlock()
	return l.saveOnce
}

func (l *memoryList[T]) setSaveOnce(o *sync.Once) {
	l.timerMut.Lock()
	defer l.timerMut.Unlock()
	l.saveOnce = o
}

func (l *memoryList[T]) WriteE(f func(l *memoryList[T]) (any, error)) (any, error) {
	s := NewState()
	s.Lock(l)
	defer s.Unlock(l)
	return f(l)
}

func (l *memoryList[T]) Write(f func(l *memoryList[T]) any) any {
	s := NewState()
	s.Lock(l)
	defer s.Unlock(l)
	return f(l)
}

func (l *memoryList[T]) ReadE(f func(l *memoryList[T]) (any, error)) (any, error) {
	s := NewState()
	s.RLock(l)
	defer s.RUnlock(l)
	return f(l)
}

func (l *memoryList[T]) Read(f func(l *memoryList[T]) any) any {
	s := NewState()
	s.RLock(l)
	defer s.RUnlock(l)
	return f(l)
}
