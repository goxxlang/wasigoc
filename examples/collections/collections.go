package main

import "fmt"

func Sum(nums []int) int {
	total := 0
	for _, n := range nums {
		total += n
	}
	return total
}

func main() {
	nums := []int{1, 2, 3, 4, 5}
	nums = append(nums, 6)
	fmt.Println("len:", len(nums))
	fmt.Println("sum:", Sum(nums))

	for i, n := range nums {
		fmt.Printf("nums[%d] = %d\n", i, n)
	}

	ages := map[string]int{"alice": 30, "bob": 25}
	ages["carol"] = 40

	if age, ok := ages["alice"]; ok {
		fmt.Println("alice is", age)
	}

	if _, ok := ages["dave"]; !ok {
		fmt.Println("dave not found")
	}

	total := 0
	for _, age := range ages {
		total += age
	}
	fmt.Println("total age:", total)
}
