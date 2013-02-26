package main

import (
	"flag"
	"fmt"
)

var fn = flag.String("data", "F1.data", "data file to analyze")

func main() {
	flag.Parse()

	data2 := readDataFile(*fn)
	fmt.Printf("%v", data2)
}
