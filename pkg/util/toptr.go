package util

func ToPtr[T any](val T) *T {
	return &val
}
