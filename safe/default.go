package safe

// Pointer returns the dereferenced value of a pointer if it is not nil.
// If the pointer is nil, it returns the default value of the type T.
// This is a generic utility function that works with pointers of ANY type of go data types.
// Results for nil pointers:
//   - int, int8, int16, int32, int64  => 0
//   - uint, uint8, uint16, uint32, uint64 => 0
//   - uintptr => 0
//   - float32, float64 => 0.0
//   - complex64, complex128 => (0+0i)
//   - byte => 0
//   - rune => 0
//   - string => ""
//   - bool => false
//   - time.Time => zero time.Time
//   - struct => struct with all fields set to their zero values recursively
//
// Below types always return nil for nil pointers, because their zero value is nil:
//   - interface{}, slices, maps, pointers, arrays, channels, functions, error => nil
func Pointer[T any](v *T) T {
	var defaultValue T
	if v == nil {
		return defaultValue
	}
	return *v
}

// Slice returns an empty slice if the input slice is nil.
// This is a generic utility function that works with slices of any type T.
// It prevents potential nil pointer dereferences when working with slices that might be nil.
// Results for nil slices:
//   - []int, []string, []float64, []struct{}, etc. => []T{}
func Slice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
