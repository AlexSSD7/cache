# cache

CDN-like, middleware memory cache for Go applications with integrated shielding and Go 1.18 Generics.

## Usage

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/AlexSSD7/cache"
)

type Person struct {
	Name string
	Age  uint32
}

func fetchPerson(name string) (*Person, error) {
	// Heavy task here, like a DB call.
	// Shielding makes the fetch functions
	// mutually exclusive, to prevent
	// unncecessary duplicate calls
	// if there is no cache in memory.

	// For the purpose of this example though,
	// we will just return sample objects.

	switch name {
	case "John":
		return &Person{Name: "John", Age: 27}, nil
	case "Steve":
		return &Person{Name: "Steve", Age: 32}, nil
	default:
		return nil, fmt.Errorf("unknown person with name '%v'", name)
	}
}

func main() {
	// Create a new ShieldedCache instance with 5-second GC interval.
	cache := cache.NewShieldedCache[*Person](time.Second * 5)

	cacheCtx, cacheCtxCancel := context.WithCancel(context.Background())
	defer cacheCtxCancel()

	// Worker is a separate goroutine running in background.
	// It is responsible for evicting old, expired objects.
	cache.StartWorker(cacheCtx)

	// If there is "person_john" object in cache, return it. Otherwise, execute fetchPerson("John").
	ret, hit, err := cache.Fetch("person_john", time.Minute, func() (*Person, error) {
		return fetchPerson("John")
	})

	fmt.Printf("result: %+v, hit: %v, err: %v\n", ret.Data, hit, err)
	// result: &{Name:John Age:27}, hit: false, err: <nil>

	ret, hit, err = cache.Fetch("person_john", time.Minute, func() (*Person, error) {
		return fetchPerson("John")
	})

	fmt.Printf("result: %+v, hit: %v, err: %v\n", ret.Data, hit, err)
	// result: &{Name:John Age:27}, hit: true, err: <nil>

	ret, hit, err = cache.Fetch("person_steve", time.Minute, func() (*Person, error) {
		return fetchPerson("Steve")
	})

	fmt.Printf("result: %+v, hit: %v, err: %v\n", ret.Data, hit, err)
	// result: &{Name:Steve Age:32}, hit: false, err: <nil>

	ret, hit, err = cache.Fetch("person_alice", time.Minute, func() (*Person, error) {
		return fetchPerson("Alice")
	})

	fmt.Printf("result: %+v, hit: %v, err: %v\n", ret, hit, err)
	// result: <nil>, hit: false, err: unknown person with name 'Alice'
}
```

# License

MIT