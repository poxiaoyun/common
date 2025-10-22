package meta

// Map applies the function f to each element of the input slice and returns a new slice with the results.
// example:
//
//	squaredfunc := func(x int) int { return x * x }
//	squared := Map([]int{1, 2, 3, 4}, squaredfunc)
//	// squared is now []int{1, 4, 9, 16}
func Map[T, R any](input []T, f func(T) R) []R {
	output := make([]R, len(input))
	for i, v := range input {
		output[i] = f(v)
	}
	return output
}

// Reduce reduces the input slice to a single value by applying the function f to each element and an accumulator.
// example:
//
//	sumfunc := func(acc int, x int) int { return acc + x }
//	sum := Reduce([]int{1, 2, 3, 4}, 0, sumfunc)
//	// sum is now 10
func Reduce[T, R any](input []T, initial R, f func(R, T) R) R {
	result := initial
	for _, v := range input {
		result = f(result, v)
	}
	return result
}
