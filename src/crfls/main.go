package main

import (
	"flag"
	"fmt"

	"crfl/src/crfl"
)

func main() {
	port := flag.Int("p", 3862, "port to listen on")
	maxListeners := flag.Int("n", 1, "max listeners")
	verbose := flag.Bool("v", false, "verbose mode")
	flag.Parse()

	if err := crfl.NewServer(*port, *maxListeners).Start(*verbose); err != nil {
		fmt.Printf("error: %v\n", err)
	}
}
