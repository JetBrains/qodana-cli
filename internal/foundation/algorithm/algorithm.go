package algorithm

import "slices"

// AppendUnique appends elements to a slice only if they are not already present.
func AppendUnique[T comparable](slice []T, elems ...T) []T {
	for _, e := range elems {
		if !slices.Contains(slice, e) {
			slice = append(slice, e)
		}
	}
	return slice
}

func Filter[T any](iterable []T, predicate func(T) bool) []T {
	var result []T
	for _, item := range iterable {
		if predicate(item) {
			result = append(result, item)
		}
	}

	return result
}

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
