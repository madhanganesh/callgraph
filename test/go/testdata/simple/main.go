package main

import "fmt"

func main() {
	a := process()
	b := shortcut()
	fmt.Println(a, b)
}

func process() int {
	return compute(42)
}

// shortcut also calls compute — so compute has two callers.
func shortcut() int {
	return compute(7) + 100
}

func compute(n int) int {
	return helper(n) + 1
}

func helper(n int) int {
	return n * 2
}
