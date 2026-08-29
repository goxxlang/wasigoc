package main

import (
	"flag"
	"fmt"
)

func main() {
	name := flag.String("name", "world", "a name")
	count := flag.Int("count", 1, "a count")
	verbose := flag.Bool("verbose", false, "verbose")

	flag.Parse()

	fmt.Println(*name, *count, *verbose)
	fmt.Println(flag.NArg())
	for i := 0; i < flag.NArg(); i++ {
		fmt.Println(flag.Arg(i))
	}
}
