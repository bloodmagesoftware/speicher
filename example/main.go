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
	foo, err := speicher.LoadMap[Foo]("./data/foo.json")
	if err != nil {
		panic(err)
	}

	func() {
		tx := foo.Begin()
		defer tx.Rollback()
		foo.Set("a", Foo{"aaa", 42})
		foo.Set("b", Foo{"abc", 69})
		foo.Set("c", Foo{"def", 420})
		tx.Commit()
	}()

	func() {
		for key, value := range foo.Select().Limit(2) { // use the Select method to create an iterator
			fmt.Printf("%s => (%s, %d)\n", key, value.Bar, value.Baz)
		}
	}()

	func() {
		tx := foo.Begin()
		defer tx.Rollback()
		a, ok := tx.Get("a")
		if ok {
			a.Baz *= 10 // a.Baz only gets modified because the transaction uses a pointer
		}
		tx.Commit()
	}()

	func() {
		a, ok := foo.Get("a")
		if ok {
			fmt.Printf("changed a => (%s, %d)\n", a.Bar, a.Baz)
		}
	}()
}
