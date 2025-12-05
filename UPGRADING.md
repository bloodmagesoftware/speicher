# Upgrading to speicher v2

This guide helps you migrate from speicher v1 to v2. Version 2 introduces a breaking API change to fix deadlock issues with the original locking design.

## Why the Change?

In v1, locks were acquired directly on stores via `store.Lock()` and `store.Unlock()`. This design was prone to deadlocks when:

- A goroutine needed to lock multiple stores
- A goroutine needed to upgrade from a read lock to a write lock
- Recursive locking was attempted

Version 2 introduces a `State` object that manages locks per goroutine, enabling:

- Recursive locking (calling Lock multiple times without deadlock)
- Lock upgrading (going from read to write lock)
- Safe multi-store locking
- Automatic lock downgrading when appropriate

## Migration Guide

### Basic Lock Changes

**v1 (old):**
```go
foo.Lock()
defer foo.Unlock()
foo.Set("key", value)
```

**v2 (new):**
```go
s := speicher.NewState()
s.Lock(foo)
defer s.Unlock(foo)
foo.Set("key", value)
```

### Read Lock Changes

**v1 (old):**
```go
foo.RLock()
defer foo.RUnlock()
value, ok := foo.Get("key")
```

**v2 (new):**
```go
s := speicher.NewState()
s.RLock(foo)
defer s.RUnlock(foo)
value, ok := foo.Get("key")
```

### Multiple Stores

With v2, you can safely lock multiple stores with a single State:

```go
s := speicher.NewState()

// Lock both stores
s.Lock(users)
s.Lock(orders)
defer s.Unlock(users)
defer s.Unlock(orders)

// Work with both stores safely
user, _ := users.Get("user1")
orders.Set("order1", newOrder)
```

### Recursive Locking

v2 supports recursive locking without deadlocks:

```go
s := speicher.NewState()

s.RLock(foo)  // First read lock
s.RLock(foo)  // Second read lock (counted, no deadlock)
s.Lock(foo)   // Upgrade to write lock
s.RLock(foo)  // Additional read while holding write

// Do work...

s.RUnlock(foo) // Decrement read count
s.Unlock(foo)  // Release write, auto-downgrades to read
s.RUnlock(foo) // Decrement read count
s.RUnlock(foo) // Final unlock, mutex released
```

### Using Helper Functions

The helper functions `Write`, `WriteE`, `Read`, and `ReadE` continue to work but now create their own State internally:

```go
// These still work as before
speicher.Write(foo, func(m speicher.Map[*Foo]) struct{} {
    m.Set("key", value)
    return struct{}{}
})

result := speicher.Read(foo, func(m speicher.Map[*Foo]) string {
    if v, ok := m.Get("key"); ok {
        return v.Name
    }
    return ""
})
```

### State Best Practices

1. **One State per goroutine**: Each goroutine should create its own State. Never share State objects between goroutines.

2. **Create State at the start of operations**: Create a new State when beginning a logical unit of work.

3. **Balance locks**: Every `Lock` needs an `Unlock`, every `RLock` needs an `RUnlock`.

4. **Use defer for cleanup**: Always use `defer s.Unlock(store)` to ensure locks are released.

## API Changes Summary

### Removed from Map and List interfaces

- `Lock()`
- `Unlock()`
- `RLock()`
- `RUnlock()`

### Added

- `speicher.NewState() *State` - Creates a new State for lock management
- `(*State).Lock(store)` - Acquires write lock
- `(*State).Unlock(store)` - Releases write lock
- `(*State).RLock(store)` - Acquires read lock
- `(*State).RUnlock(store)` - Releases read lock
- `(*State).HasReadLock(store) bool` - Checks if holding read lock
- `(*State).HasWriteLock(store) bool` - Checks if holding write lock

### Unchanged

- `LoadMap[T](location string) (Map[T], error)`
- `LoadList[T](location string) (List[T], error)`
- All data access methods (`Get`, `Set`, `Delete`, `Find`, `Iterate`, etc.)
- `Save() error`
- Helper functions (`Write`, `WriteE`, `Read`, `ReadE`)

## Common Migration Patterns

### Before (v1)

```go
func processData(store speicher.Map[*Data]) error {
    store.Lock()
    defer store.Unlock()
    
    // ... work with store
    return nil
}
```

### After (v2)

```go
func processData(store speicher.Map[*Data]) error {
    s := speicher.NewState()
    s.Lock(store)
    defer s.Unlock(store)
    
    // ... work with store
    return nil
}
```

### Passing State to Functions

For complex operations spanning multiple functions, pass the State:

```go
func main() {
    s := speicher.NewState()
    s.Lock(users)
    defer s.Unlock(users)
    
    processUser(s, users, "user1")
}

func processUser(s *speicher.State, users speicher.Map[*User], id string) {
    // State already holds the lock, no need to lock again
    // But we can if we want recursive locking
    user, ok := users.Get(id)
    // ...
}
```


