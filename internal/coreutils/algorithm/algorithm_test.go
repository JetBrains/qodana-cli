package algorithm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilter(t *testing.T) {
	t.Run("filter even numbers", func(t *testing.T) {
		nums := []int{1, 2, 3, 4, 5, 6}
		result := Filter(nums, func(n int) bool { return n%2 == 0 })
		assert.Equal(t, []int{2, 4, 6}, result)
	})

	t.Run("filter strings by length", func(t *testing.T) {
		strs := []string{"a", "bb", "ccc", "d"}
		result := Filter(strs, func(s string) bool { return len(s) > 1 })
		assert.Equal(t, []string{"bb", "ccc"}, result)
	})

	t.Run("filter all", func(t *testing.T) {
		nums := []int{1, 2, 3}
		result := Filter(nums, func(n int) bool { return false })
		assert.Empty(t, result)
	})

	t.Run("filter none", func(t *testing.T) {
		nums := []int{1, 2, 3}
		result := Filter(nums, func(n int) bool { return true })
		assert.Equal(t, []int{1, 2, 3}, result)
	})

	t.Run("empty slice", func(t *testing.T) {
		nums := []int{}
		result := Filter(nums, func(n int) bool { return true })
		assert.Empty(t, result)
	})
}

func TestUnique(t *testing.T) {
	t.Run("remove duplicates", func(t *testing.T) {
		nums := []int{1, 2, 2, 3, 3, 3, 4}
		result := Unique(nums)
		assert.Equal(t, []int{1, 2, 3, 4}, result)
	})

	t.Run("no duplicates", func(t *testing.T) {
		nums := []int{1, 2, 3}
		result := Unique(nums)
		assert.Equal(t, []int{1, 2, 3}, result)
	})

	t.Run("all duplicates", func(t *testing.T) {
		nums := []int{1, 1, 1}
		result := Unique(nums)
		assert.Equal(t, []int{1}, result)
	})

	t.Run("strings", func(t *testing.T) {
		strs := []string{"a", "b", "a", "c", "b"}
		result := Unique(strs)
		assert.Equal(t, []string{"a", "b", "c"}, result)
	})

	t.Run("empty slice", func(t *testing.T) {
		nums := []int{}
		result := Unique(nums)
		assert.Empty(t, result)
	})
}

