package util

func Coa[T comparable](v, fallback T) T {
	if v == *new(T) {
		return fallback
	}
	return v
}

func CoaFunc[T, U comparable](fn func(v T) (U, error), v T, fallback U) (U, error) {
	if v == *new(T) {
		return fallback, nil
	}

	return fn(v)
}
