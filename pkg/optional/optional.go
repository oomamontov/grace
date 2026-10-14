// Package optional provides a generic value that may or may not be set.
package optional

// Value represents generic value that may or may not be set.
// Just like pointer with nil or non-nil value, but without indirection.
// Value is not goroutine-safe.
type Value[T any] struct {
	value T
	isSet bool
}

// Empty returns an unset value.
func Empty[T any]() Value[T] {
	return Value[T]{}
}

// New returns a value set to val.
func New[T any](val T) Value[T] {
	return Value[T]{
		value: val,
		isSet: true,
	}
}

// Set sets the value to val.
func (v *Value[T]) Set(val T) {
	v.value = val
	v.isSet = true
}

// Unset resets the value to the unset state.
func (v *Value[T]) Unset() {
	v.isSet = false

	var zero T

	v.value = zero
}

// SetIfUnset sets the value to val unless it is already set.
func (v *Value[T]) SetIfUnset(val T) {
	if v.isSet {
		return
	}

	v.Set(val)
}

// Or returns the value if it is set, and val otherwise.
func (v Value[T]) Or(val T) T {
	if v.isSet {
		return v.value
	}

	return val
}

// OrOther returns the first set value among v and others, or an unset value if none is set.
func (v Value[T]) OrOther(others ...Value[T]) Value[T] {
	res := v
	for _, other := range others {
		if res.isSet {
			return res
		}

		res = other
	}

	return res
}

// Get returns the value and reports whether it is set.
func (v Value[T]) Get() (T, bool) {
	return v.value, v.isSet
}

// ShouldGet returns the value and panics if it is not set.
func (v Value[T]) ShouldGet() T {
	if !v.isSet {
		panic("value is not set")
	}

	return v.value
}

// GetOrDefault returns the value if it is set, and the zero value otherwise.
func (v Value[T]) GetOrDefault() T {
	return v.value
}

// IsSet reports whether the value is set.
func (v Value[T]) IsSet() bool {
	return v.isSet
}
