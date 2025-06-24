package speicher

type Store interface {
	// lock acquires the write lock for the data store to allow safe updates.
	// Don't forget to use Unlock when you are done.
	lock()

	// unlock releases the write lock for the data store.
	unlock()

	// rLock acquires the read lock for the data store to allow safe reading.
	// Don't forget to use RUnlock when you are done.
	rLock()

	// rUnlock releases the read lock for the data store.
	rUnlock()
}

// Same as Write but f returns an error.
func WriteE[S Store, R any](s Store, f func(s Store) (R, error)) (R, error) {
	s.lock()
	defer s.unlock()
	return f(s)
}

// Write locks the store before executing f. After f was executed, the store gets unlocked.
func Write[S Store, R any](s Store, f func(s Store) R) R {
	s.lock()
	defer s.unlock()
	return f(s)
}
