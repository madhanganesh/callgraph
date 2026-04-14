package main

func compute(n int) int {
	return helper(n) + 1
}

func helper(n int) int {
	return n * 2
}

func alsoCallsCompute() int {
	return compute(7)
}
