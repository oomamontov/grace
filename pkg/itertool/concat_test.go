package itertool

import (
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConcat(t *testing.T) {
	t.Parallel()

	seq := Concat(slices.Values([]int{1, 2}), slices.Values([]int(nil)), slices.Values([]int{3}))
	require.Equal(t, []int{1, 2, 3}, slices.Collect(seq))

	var first []int

	for v := range seq {
		first = append(first, v)
		if v == 2 {
			break
		}
	}

	require.Equal(t, []int{1, 2}, first)
}

func TestConcat2(t *testing.T) {
	t.Parallel()

	seq := Concat2(maps.All(map[string]int{"a": 1}), maps.All(map[string]int{"b": 2}))
	require.Equal(t, map[string]int{"a": 1, "b": 2}, maps.Collect(seq))

	count := 0

	for range seq {
		count++

		break
	}

	require.Equal(t, 1, count)
}
