// non-sucking database
//
// speicher v2 uses a State object for lock management to prevent deadlocks.
// Each goroutine should create its own State and use it for all lock operations.
//
// Example:
//
//	package main
//
//	import (
//		"fmt"
//
//		"github.com/bloodmagesoftware/speicher/v2"
//	)
//
//	type Foo struct {
//		Bar string
//		Baz int
//	}
//
//	func main() {
//		foo, err := speicher.LoadMap[*Foo]("./data/foo.json")
//		if err != nil {
//			panic(err)
//		}
//
//		func() {
//			s := speicher.NewState()
//			s.Lock(foo) // use Lock to get write access
//			defer s.Unlock(foo) // use Unlock to release write access
//			foo.Set("a", &Foo{"aaa", 42})
//			foo.Set("b", &Foo{"abc", 69})
//		}()
//
//		func() {
//			s := speicher.NewState()
//			s.RLock(foo) // use RLock to get read access
//			defer s.RUnlock(foo) // use RUnlock to release read access
//			for key, value := range foo.Iterate { // use the Iterate method to create an iterator
//				fmt.Printf("%s => (%s, %d)\n", key, value.Bar, value.Baz)
//			}
//		}()
//
//		func() {
//			s := speicher.NewState()
//			s.Lock(foo)
//			defer s.Unlock(foo)
//			a, ok := foo.Get("a")
//			if ok {
//				a.Baz *= 10 // a.Baz only gets modified because the store uses a pointer
//			}
//		}()
//
//		func() {
//			s := speicher.NewState()
//			s.RLock(foo)
//			defer s.RUnlock(foo)
//			a, ok := foo.Get("a")
//			if ok {
//				fmt.Printf("changed a => (%s, %d)\n", a.Bar, a.Baz)
//			}
//		}()
//	}
package speicher
