package optional

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestValue(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name    string
		prepare func() Value[int]
		wantVal int
		wantOk  bool
	}{
		{
			name: "default value",
			prepare: func() Value[int] {
				return Value[int]{}
			},
			wantOk: false,
		},
		{
			name: "empty value",
			prepare: func() Value[int] {
				return Empty[int]()
			},
			wantOk: false,
		},
		{
			name: "new value",
			prepare: func() Value[int] {
				return New(1)
			},
			wantVal: 1,
			wantOk:  true,
		},
		{
			name: "once set default value",
			prepare: func() Value[int] {
				var res Value[int]
				var val int
				res.Set(val)
				return res
			},
			wantOk: true,
		},
		{
			name: "once set value",
			prepare: func() Value[int] {
				var res Value[int]
				res.Set(1)
				return res
			},
			wantVal: 1,
			wantOk:  true,
		},
		{
			name: "twice set value",
			prepare: func() Value[int] {
				var res Value[int]
				res.Set(1)
				res.Set(2)
				return res
			},
			wantVal: 2,
			wantOk:  true,
		},
		{
			name: "unset value",
			prepare: func() Value[int] {
				var res Value[int]
				res.Unset()
				return res
			},
			wantOk: false,
		},
		{
			name: "unset after set",
			prepare: func() Value[int] {
				res := New(1)
				res.Unset()
				return res
			},
			wantOk: false,
		},
		{
			name: "set if unset on empty",
			prepare: func() Value[int] {
				res := Empty[int]()
				res.SetIfUnset(1)
				return res
			},
			wantVal: 1,
			wantOk:  true,
		},
		{
			name: "set if unset on new",
			prepare: func() Value[int] {
				res := New(1)
				res.SetIfUnset(2)
				return res
			},
			wantVal: 1,
			wantOk:  true,
		},
		{
			name: "set if unset on unset",
			prepare: func() Value[int] {
				res := New(1)
				res.Unset()
				res.SetIfUnset(2)
				return res
			},
			wantVal: 2,
			wantOk:  true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			val := tc.prepare()
			gotVal, gotOk := val.Get()
			require.Equal(t, tc.wantVal, gotVal, "unexpected value")
			require.Equal(t, tc.wantOk, gotOk, "unexpected set state")
		})
	}
}

func TestValue_ShouldGet(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name        string
		prepare     func() Value[int]
		want        int
		shouldPanic bool
	}{
		{
			name: "empty",
			prepare: func() Value[int] {
				return Empty[int]()
			},
			shouldPanic: true,
		},
		{
			name: "non-empty",
			prepare: func() Value[int] {
				return New(1)
			},
			want: 1,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			val := tc.prepare()
			defer func() {
				r := recover()
				if tc.shouldPanic {
					require.NotNil(t, r, "excepted panic")
				} else {
					require.Nil(t, r, "unexpected panic")
				}
			}()
			res := val.ShouldGet()
			require.Equal(t, tc.want, res)
		})
	}
}

func TestValue_GetOrDefault(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name    string
		prepare func() Value[int]
		want    int
	}{
		{
			name: "empty",
			prepare: func() Value[int] {
				return Empty[int]()
			},
		},
		{
			name: "non-empty",
			prepare: func() Value[int] {
				return New(1)
			},
			want: 1,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			val := tc.prepare()
			res := val.GetOrDefault()
			require.Equal(t, tc.want, res)
		})
	}
}
