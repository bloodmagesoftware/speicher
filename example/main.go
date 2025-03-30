package main

import (
	"fmt"

	"github.com/bloodmagesoftware/speicher"
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

	func() {
		foo.Lock()
		defer foo.Unlock()
		foo.Set("a", &Foo{"aaa", 42})
		foo.Set("b", &Foo{"abc", 69})
	}()

	func() {
		foo.RLock()
		defer foo.RUnlock()
		ch, close := foo.RangeKV()
		defer close()
		for el := range ch {
			fmt.Printf("%s => (%s, %d)\n", el.Key, el.Value.Bar, el.Value.Baz)
		}
	}()

	func() {
		foo.Lock()
		defer foo.Unlock()
		a, ok := foo.Get("a")
		if ok {
			a.Baz *= 10
		}
	}()

	func() {
		foo.RLock()
		defer foo.RUnlock()
		a, ok := foo.Get("a")
		if ok {
			fmt.Printf("changed a => (%s, %d)\n", a.Bar, a.Baz)
		}
	}()
}
