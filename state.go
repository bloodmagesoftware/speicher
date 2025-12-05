package speicher

import (
	"sync"
	"sync/atomic"
)

// storeID is a unique identifier for each store instance.
type storeID uint64

var nextStoreID atomic.Uint64

func newStoreID() storeID {
	return storeID(nextStoreID.Add(1))
}

// lockable is the internal interface that stores must implement for State to manage their locks.
type lockable interface {
	getStoreID() storeID
	getMutex() *sync.RWMutex
}

// lockState tracks the lock counts for a single store within a State.
type lockState struct {
	readCount  int
	writeCount int
}

// State manages lock state for multiple stores within a single goroutine.
// It tracks recursive read and write locks and handles lock upgrading.
//
// A State must not be shared across goroutines. Each goroutine that needs
// to access stores should create its own State via NewState().
//
// Example usage:
//
//	s := speicher.NewState()
//	s.Lock(myMap)
//	defer s.Unlock(myMap)
//	myMap.Set("key", value)
type State struct {
	locks map[storeID]*lockState
}

// NewState creates a new State for managing locks.
// Each goroutine should have its own State instance.
func NewState() *State {
	return &State{
		locks: make(map[storeID]*lockState),
	}
}

func (s *State) getLockState(id storeID) *lockState {
	ls, ok := s.locks[id]
	if !ok {
		ls = &lockState{}
		s.locks[id] = ls
	}
	return ls
}

// Lock acquires a write lock on the store.
// If the State already holds a write lock on this store, the lock count is incremented.
// If the State holds read locks, they are upgraded to a write lock.
//
// Multiple calls to Lock must be balanced with equal calls to Unlock.
func (s *State) Lock(store lockable) {
	id := store.getStoreID()
	mut := store.getMutex()
	ls := s.getLockState(id)

	if ls.writeCount > 0 {
		// Already have write lock, just increment
		ls.writeCount++
		return
	}

	if ls.readCount > 0 {
		// Need to upgrade: release read lock first, then acquire write lock
		mut.RUnlock()
	}

	mut.Lock()
	ls.writeCount++
}

// Unlock releases a write lock on the store.
// If there are pending read locks from before the write lock was acquired,
// the mutex downgrades to a read lock.
//
// When the write lock is fully released, automatic save is triggered for
// stores that support persistence.
//
// Panics if called without a matching Lock call.
func (s *State) Unlock(store lockable) {
	id := store.getStoreID()
	mut := store.getMutex()
	ls := s.getLockState(id)

	if ls.writeCount == 0 {
		panic("speicher: Unlock called without matching Lock")
	}

	ls.writeCount--

	if ls.writeCount == 0 {
		// Release the write lock
		mut.Unlock()

		// Notify that the store was changed (triggers auto-save)
		if sav, ok := store.(savable); ok {
			notifyChanged(sav)
		}

		// If we had read locks before upgrading, re-acquire read lock
		if ls.readCount > 0 {
			mut.RLock()
		}
	}
}

// RLock acquires a read lock on the store.
// If the State already holds a write lock, the read is implicitly satisfied
// without additional mutex operations.
// If the State already holds read locks, the count is incremented.
//
// Multiple calls to RLock must be balanced with equal calls to RUnlock.
func (s *State) RLock(store lockable) {
	id := store.getStoreID()
	mut := store.getMutex()
	ls := s.getLockState(id)

	if ls.writeCount > 0 {
		// Already have write lock, read is implicitly satisfied
		ls.readCount++
		return
	}

	if ls.readCount == 0 {
		// First read lock, acquire it
		mut.RLock()
	}

	ls.readCount++
}

// RUnlock releases a read lock on the store.
// The actual mutex is only released when all read and write locks are released.
//
// Panics if called without a matching RLock call.
func (s *State) RUnlock(store lockable) {
	id := store.getStoreID()
	mut := store.getMutex()
	ls := s.getLockState(id)

	if ls.readCount == 0 {
		panic("speicher: RUnlock called without matching RLock")
	}

	ls.readCount--

	// Only release the mutex if:
	// - No more read locks AND
	// - No write locks (if there's a write lock, it owns the mutex)
	if ls.readCount == 0 && ls.writeCount == 0 {
		mut.RUnlock()
	}
}

// HasReadLock returns true if the State holds at least one read lock on the store.
func (s *State) HasReadLock(store lockable) bool {
	id := store.getStoreID()
	ls, ok := s.locks[id]
	if !ok {
		return false
	}
	return ls.readCount > 0 || ls.writeCount > 0
}

// HasWriteLock returns true if the State holds at least one write lock on the store.
func (s *State) HasWriteLock(store lockable) bool {
	id := store.getStoreID()
	ls, ok := s.locks[id]
	if !ok {
		return false
	}
	return ls.writeCount > 0
}

