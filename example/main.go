package main

import (
	"fmt"

	"github.com/bloodmagesoftware/speicher/v2"
)

type Foo struct {
	Bar string
	Baz int
}

func main() {
	foo, err := speicher.LoadMap[*Foo]("./data/foo.json")
	if err != nil {
		panic(err)
	}

	// Write example: using State for write lock
	func() {
		s := speicher.NewState()
		s.Lock(foo)
		defer s.Unlock(foo)
		foo.Set("a", &Foo{"aaa", 42})
		foo.Set("b", &Foo{"abc", 69})
	}()

	// Read example: using State for read lock with iteration
	func() {
		s := speicher.NewState()
		s.RLock(foo)
		defer s.RUnlock(foo)
		for key, value := range foo.Iterate {
			fmt.Printf("%s => (%s, %d)\n", key, value.Bar, value.Baz)
		}
	}()

	// Modify via pointer (requires write lock since the map holds pointers)
	func() {
		s := speicher.NewState()
		s.Lock(foo)
		defer s.Unlock(foo)
		a, ok := foo.Get("a")
		if ok {
			a.Baz *= 10
		}
	}()

	// Read modified value
	func() {
		s := speicher.NewState()
		s.RLock(foo)
		defer s.RUnlock(foo)
		a, ok := foo.Get("a")
		if ok {
			fmt.Printf("changed a => (%s, %d)\n", a.Bar, a.Baz)
		}
	}()

	// Example: recursive locking (no deadlock)
	func() {
		s := speicher.NewState()
		s.RLock(foo) // First read lock
		s.RLock(foo) // Second read lock (recursive)
		s.Lock(foo)  // Upgrade to write lock
		s.RLock(foo) // Additional read lock while holding write

		foo.Set("c", &Foo{"recursive", 100})

		s.RUnlock(foo) // Release one read
		s.Unlock(foo)  // Release write, downgrades back to read
		s.RUnlock(foo) // Release second read
		s.RUnlock(foo) // Release first read
	}()

	// Using the helper function for simple transactions
	speicher.Write(foo, func(m speicher.Map[*Foo]) struct{} {
		m.Set("d", &Foo{"via helper", 200})
		return struct{}{}
	})

	// Read using helper
	result := speicher.Read(foo, func(m speicher.Map[*Foo]) string {
		if d, ok := m.Get("d"); ok {
			return fmt.Sprintf("d => (%s, %d)", d.Bar, d.Baz)
		}
		return "not found"
	})
	fmt.Println(result)
}
