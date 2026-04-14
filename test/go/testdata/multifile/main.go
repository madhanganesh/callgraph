package main

import "fmt"

func main() {
	fmt.Println(process())
}

func process() int {
	return compute(42)
}
