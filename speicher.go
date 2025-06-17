// non-sucking database
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
//			tsx := foo.Begin()
//			defer tsx.Commit()
//			foo.Set("a", &Foo{"aaa", 42})
//			foo.Set("b", &Foo{"abc", 69})
//		}()
//
//		func() {
//			for key, value := range foo.Iterate { // use the Iterate method to create an iterator
//				fmt.Printf("%s => (%s, %d)\n", key, value.Bar, value.Baz)
//			}
//		}()
//
//		func() {
//			tsx := foo.Begin()
//			defer tsx.Commit()
//			a, ok := tsx.Get("a")
//			if ok {
//				a.Baz *= 10 // a.Baz only gets modified because the store uses a pointer
//			}
//		}()
//
//		func() {
//			a, ok := foo.Get("a")
//			if ok {
//				fmt.Printf("changed a => (%s, %d)\n", a.Bar, a.Baz)
//			}
//		}()
//	}
package speicher
