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
	// memoryList is a List implementation that keeps all elements in memory.
	memoryList[T any] struct {
		data     []T
		location string
		mut      sync.RWMutex

		timerMut     sync.Mutex
		saveTimer    *time.Timer
		maxSaveTimer *time.Timer
		saveOnce     *sync.Once
		clone        func(T) T
	}

	ListIterator[T any] func(yield func(v T) bool)

	ListSelector[T any] func(v T) bool

	ListOrderBy[T any] func(a T, b T) bool

	// List is a thread-safe list data store interface that provides basic
	// CRUD operations, predicate-based search, and iteration functionality.
	List[T any] interface {
		// Get returns the value at a given index of the List and a bool that indicates whether the index exists or not.
		// If no element is found, the bool result will be false.
		Get(index int) (T, bool)

		// Find traverses the List and returns the first element that satisfies the provided predicate function.
		// If no element is found, the bool result will be false.
		Find(func(T) bool) (value T, found bool)

		// FindAll returns all elements in the List that satisfy the provided predicate function.
		// If no elements match, it returns an empty slice.
		FindAll(func(T) bool) (values []T)

		// Append adds the provided value to the end of the List.
		Append(value T)

		// AppendUnique adds the provided value to the List only if no existing element is equal to it,
		// based on the supplied equality function. It returns true if the value was added,
		// and false otherwise.
		AppendUnique(value T, equal func(a, b T) bool) bool

		// Set assigns the provided value to the element at the specified index.
		// If the index is out of bounds, it returns an error.
		Set(index int, value T) error

		// Overwrite replaces the entire List with the data provided in the slice.
		Overwrite([]T)

		// Len returns the number of elements currently in the List.
		Len() int

		// Select iterates over the List and calls the provided function for each element.
		Select() ListIterator[T]

		// Save persists the current state of the List to its underlying data store.
		// It returns an error if the operation fails.
		Save() error

		// Write executes the given function while holding exclusive write lock.
		Write(f func(l *memoryList[T]) any) any

		// WriteE executes the given function while holding exclusive write lock.
		WriteE(f func(l *memoryList[T]) (any, error)) (any, error)
	}
)

func (l *memoryList[T]) Get(index int) (value T, found bool) {
	l.mut.RLock()
	defer l.mut.RUnlock()
	if index < 0 || index >= len(l.data) {
		value = l.clone(l.data[index])
		found = true
	} else {
		found = false
	}
	return
}

func (l *memoryList[T]) Append(value T) {
	l.mut.Lock()
	defer l.mut.Unlock()
	l.data = append(l.data, value)
}

func (l *memoryList[T]) AppendUnique(value T, equal func(a, b T) bool) bool {
	l.mut.Lock()
	defer l.mut.Unlock()
	for _, x := range l.data {
		if equal(x, value) {
			return false
		}
	}
	l.data = append(l.data, value)
	return true
}

func (l *memoryList[T]) Find(f func(T) bool) (value T, found bool) {
	l.mut.RLock()
	defer l.mut.RUnlock()
	for _, v := range l.data {
		if f(v) {
			found = true
			value = l.clone(v)
			return
		}
	}
	found = false
	return
}

func (l *memoryList[T]) FindAll(f func(T) bool) (values []T) {
	l.mut.RLock()
	defer l.mut.RUnlock()
	for _, value := range l.data {
		if f(value) {
			values = append(values, l.clone(value))
		}
	}
	return
}

func (l *memoryList[T]) Set(index int, value T) error {
	l.mut.Lock()
	defer l.mut.Unlock()
	if index < 0 || index >= len(l.data) {
		return fmt.Errorf("index out of range")
	}
	l.data[index] = value
	return nil
}

func (l *memoryList[T]) Overwrite(values []T) {
	l.mut.Lock()
	defer l.mut.Unlock()
	l.data = values
}

func (l *memoryList[T]) Len() int {
	l.mut.RLock()
	defer l.mut.RUnlock()
	return len(l.data)
}

func (l *memoryList[T]) Select() ListIterator[T] {
	return func(yield func(v T) bool) {
		l.mut.RLock()
		defer l.mut.RUnlock()
		for _, value := range l.data {
			if !yield(l.clone(value)) {
				break
			}
		}
	}
}

func (l *memoryList[T]) Save() error {
	l.mut.RLock()
	defer l.mut.RUnlock()

	f, err := os.Create(l.location)
	if err != nil {
		return errors.Join(fmt.Errorf("failed to open file '%s'", l.location), err)
	}
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
		location: location,
		data:     make([]T, 0),
		clone:    clone.CopyConstructor[T](),
	}
	f, err := os.Open(location)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(filepath.Dir(location), 0740)
			return l, err
		}
		return nil, errors.Join(fmt.Errorf("failed to open file '%s' (file exists)", location), err)
	}
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
	l.mut.Lock()
	defer l.mut.Unlock()
	return f(l)
}

func (l *memoryList[T]) Write(f func(l *memoryList[T]) any) any {
	l.mut.Lock()
	defer l.mut.Unlock()
	return f(l)
}

func (li ListIterator[T]) Where(where ListSelector[T]) ListIterator[T] {
	return func(yield func(v T) bool) {
		for value := range li {
			if !where(value) {
				continue
			}
			if !yield(value) {
				break
			}
		}
	}
}

func (li ListIterator[T]) OrderBy(orderBy ListOrderBy[T]) ListIterator[T] {
	return func(yield func(v T) bool) {
		var values []T
		for value := range li {
			values = append(values, value)
		}
		sort.Slice(values, func(i, j int) bool {
			return orderBy(values[i], values[j])
		})
		for _, value := range values {
			if !yield(value) {
				break
			}
		}
	}
}

func (li ListIterator[T]) Limit(limit int) ListIterator[T] {
	return func(yield func(v T) bool) {
		i := 0
		for value := range li {
			if i >= limit {
				break
			}
			if !yield(value) {
				break
			}
			i++
		}
	}
}
