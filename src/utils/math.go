package utils

// Abs returns the absolute value of x
func Abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// Max returns the maximum of two integers
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Min returns the minimum of two integers
func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
