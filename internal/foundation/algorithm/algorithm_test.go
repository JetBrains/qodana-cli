package algorithm

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAppendUnique(t *testing.T) {
	t.Run("append new element", func(t *testing.T) {
		result := AppendUnique([]string{"a", "b"}, "c")
		assert.Equal(t, []string{"a", "b", "c"}, result)
	})

	t.Run("append existing element", func(t *testing.T) {
		result := AppendUnique([]string{"a", "b"}, "a")
		assert.Equal(t, []string{"a", "b"}, result)
	})

	t.Run("append multiple elements", func(t *testing.T) {
		result := AppendUnique([]string{"a"}, "b", "c", "a")
		assert.Equal(t, []string{"a", "b", "c"}, result)
	})

	t.Run("append to empty slice", func(t *testing.T) {
		result := AppendUnique([]string{}, "a")
		assert.Equal(t, []string{"a"}, result)
	})

	t.Run("append all duplicates", func(t *testing.T) {
		result := AppendUnique([]string{"a", "b"}, "a", "b")
		assert.Equal(t, []string{"a", "b"}, result)
	})

	t.Run("generic ints", func(t *testing.T) {
		result := AppendUnique([]int{1, 2}, 3, 2, 4)
		assert.Equal(t, []int{1, 2, 3, 4}, result)
	})
}

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

func TestForEachBounded(t *testing.T) {
	t.Run("processes all items", func(t *testing.T) {
		var count atomic.Int32
		items := []int{1, 2, 3, 4, 5}
		ForEachBounded(items, 2, func(n int) {
			count.Add(1)
		})
		assert.Equal(t, int32(5), count.Load())
	})

	t.Run("respects concurrency limit", func(t *testing.T) {
		var active atomic.Int32
		var maxActive atomic.Int32
		var mu sync.Mutex

		items := make([]int, 20)
		for i := range items {
			items[i] = i
		}

		ForEachBounded(items, 3, func(n int) {
			cur := active.Add(1)
			mu.Lock()
			if cur > maxActive.Load() {
				maxActive.Store(cur)
			}
			mu.Unlock()

			// Simulate work so goroutines overlap
			time.Sleep(time.Millisecond)

			active.Add(-1)
		})

		assert.LessOrEqual(t, maxActive.Load(), int32(3))
		assert.Equal(t, int32(0), active.Load())
	})

	t.Run("empty slice", func(t *testing.T) {
		ForEachBounded([]int{}, 3, func(n int) {
			t.Fatal("should not be called")
		})
	})

	t.Run("single item", func(t *testing.T) {
		var called bool
		ForEachBounded([]string{"hello"}, 1, func(s string) {
			assert.Equal(t, "hello", s)
			called = true
		})
		assert.True(t, called)
	})

	t.Run("zero concurrency does not deadlock", func(t *testing.T) {
		var count atomic.Int32
		ForEachBounded([]int{1, 2, 3}, 0, func(n int) {
			count.Add(1)
		})
		assert.Equal(t, int32(3), count.Load())
	})

	t.Run("negative concurrency does not deadlock", func(t *testing.T) {
		var count atomic.Int32
		ForEachBounded([]int{1, 2}, -5, func(n int) {
			count.Add(1)
		})
		assert.Equal(t, int32(2), count.Load())
	})

	t.Run("single panic is re-raised", func(t *testing.T) {
		assert.PanicsWithValue(t, "boom", func() {
			ForEachBounded([]int{1}, 2, func(n int) {
				panic("boom")
			})
		})
	})

	t.Run("multiple panics are re-raised", func(t *testing.T) {
		assert.Panics(t, func() {
			ForEachBounded([]int{1, 2, 3, 4}, 4, func(n int) {
				panic("boom")
			})
		})
	})
}

