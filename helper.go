package speicher

// Store is a common interface for all speicher data stores.
// Use State.Lock/State.Unlock/State.RLock/State.RUnlock for locking operations.
type Store interface {
	lockable
}

// WriteE acquires a write lock on the store, executes f, then releases the lock.
// Returns the result of f and any error.
func WriteE[S Store, R any](store S, f func(s S) (R, error)) (R, error) {
	state := NewState()
	state.Lock(store)
	defer state.Unlock(store)
	return f(store)
}

// Write acquires a write lock on the store, executes f, then releases the lock.
// Returns the result of f.
func Write[S Store, R any](store S, f func(s S) R) R {
	state := NewState()
	state.Lock(store)
	defer state.Unlock(store)
	return f(store)
}

// ReadE acquires a read lock on the store, executes f, then releases the lock.
// Returns the result of f and any error.
func ReadE[S Store, R any](store S, f func(s S) (R, error)) (R, error) {
	state := NewState()
	state.RLock(store)
	defer state.RUnlock(store)
	return f(store)
}

// Read acquires a read lock on the store, executes f, then releases the lock.
// Returns the result of f.
func Read[S Store, R any](store S, f func(s S) R) R {
	state := NewState()
	state.RLock(store)
	defer state.RUnlock(store)
	return f(store)
}
