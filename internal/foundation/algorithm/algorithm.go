package algorithm

import (
	"fmt"
	"slices"
	"sync"
)

// AppendUnique appends elements to a slice only if they are not already present.
func AppendUnique[T comparable](slice []T, elems ...T) []T {
	for _, e := range elems {
		if !slices.Contains(slice, e) {
			slice = append(slice, e)
		}
	}
	return slice
}

// Filter removes each item from `iterable` for which `predicate` returns `false`.
func Filter[T any](iterable []T, predicate func(T) bool) []T {
	var result []T
	for _, item := range iterable {
		if predicate(item) {
			result = append(result, item)
		}
	}

	return result
}

// Unique removes all duplicates from `iterable`, keeping first occurence only.
func Unique[T comparable](iterable []T) []T {
	seen := map[T]bool{}
	predicate := func(item T) bool {
		if seen[item] {
			return false
		}
		seen[item] = true
		return true
	}

	return Filter(iterable, predicate)
}

// ForEachBounded runs fn for each item with at most maxConcurrency goroutines.
// Blocks until all complete. Panics from fn are caught and re-panicked in the caller.
// Returns an error if maxConcurrency is not positive.
func ForEachBounded[T any](items []T, maxConcurrency int, fn func(T)) error {
	if maxConcurrency <= 0 {
		return fmt.Errorf("ForEachBounded: maxConcurrency must be > 0, got %d", maxConcurrency)
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrency)

	var mu sync.Mutex
	var panics []any

	for _, item := range items {
		wg.Add(1)
		go func(v T) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					mu.Lock()
					panics = append(panics, r)
					mu.Unlock()
				}
			}()
			fn(v)
		}(item)
	}
	wg.Wait()

	if len(panics) == 1 {
		panic(panics[0])
	}
	if len(panics) > 1 {
		panic(fmt.Sprintf("multiple panics (%d): %v", len(panics), panics))
	}
	return nil
}
