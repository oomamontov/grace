package optional

// Value represents generic value that may or may not be set.
// Just like pointer with nil or non-nil value, but without indirection.
// Value is not goroutine-safe.
type Value[T any] struct {
	value T
	isSet bool
}

func Empty[T any]() Value[T] {
	return Value[T]{}
}

func New[T any](val T) Value[T] {
	return Value[T]{
		value: val,
		isSet: true,
	}
}

func (v *Value[T]) Set(val T) {
	v.value = val
	v.isSet = true
}

func (v *Value[T]) Unset() {
	v.isSet = false
	var zero T
	v.value = zero
}

func (v *Value[T]) SetIfUnset(val T) {
	if v.isSet {
		return
	}
	v.Set(val)
}

func (v Value[T]) Or(val T) T {
	if v.isSet {
		return v.value
	}
	return val
}

func (v Value[T]) Get() (T, bool) {
	return v.value, v.isSet
}

func (v Value[T]) ShouldGet() T {
	if !v.isSet {
		panic("value is not set")
	}
	return v.value
}

func (v Value[T]) GetOrDefault() T {
	return v.value
}

func (v Value[T]) IsSet() bool {
	return v.isSet
}
