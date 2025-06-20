package sqlite
// IntPtr is a helper to get a pointer to an int for PaginationOptions.Offset
func IntPtr(i int) *int {
	return &i
}
