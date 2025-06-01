// non-sucking database
//
// Example:
//
//	package main
//
//	import (
//		"fmt"
//
//		"github.com/bloodmagesoftware/speicher"
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
//			foo.Lock() // use Lock to get write access
//			defer foo.Unlock() // use Unlock to release write access
//			foo.Set("a", &Foo{"aaa", 42})
//			foo.Set("b", &Foo{"abc", 69})
//		}()
//
//		func() {
//			foo.RLock() // use RLock to get read access
//			defer foo.RUnlock() // use RUnlock to release read access
//			for key, value := range foo.Iterate { // use the Iterate method to create an iterator
//				fmt.Printf("%s => (%s, %d)\n", key, value.Bar, value.Baz)
//			}
//		}()
//
//		func() {
//			foo.Lock()
//			defer foo.Unlock()
//			a, ok := foo.Get("a")
//			if ok {
//				a.Baz *= 10 // a.Baz only gets modified because the store uses a pointer
//			}
//		}()
//
//		func() {
//			foo.RLock()
//			defer foo.RUnlock()
//			a, ok := foo.Get("a")
//			if ok {
//				fmt.Printf("changed a => (%s, %d)\n", a.Bar, a.Baz)
//			}
//		}()
//	}
package speicher
