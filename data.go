package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
)

type Eqn func([]float64) float64

type Data struct {
	input  [][]float64
	output []float64

	var_names []string
	out_name  string
}

func (d *Data) length() int {
	return len(d.input)
}
func (d *Data) dimensions() int {
	return len(d.input[0])
}

func readDataFile(filename string) (d *Data) {
	d = new(Data)
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Println("Error opening file: ", filename)
		return
	}
	lines := bytes.Split(data, []byte{'\n'})
	names := bytes.Fields(lines[0])
	for i := 0; i < len(names)-1; i++ {
		d.var_names = append(d.var_names, string(names[i]))
	}
	d.out_name = string(names[len(names)-1])

	for i := 1; i < len(lines); i++ {
		val_strs := bytes.Fields(lines[i])
		if len(val_strs) < len(d.var_names)+1 {
			break
		}
		input := make([]float64, len(d.var_names))

		for p := 0; p < len(d.var_names); p++ {
			var val float64
			fmt.Sscanf(string(val_strs[p]), "%f", &val)
			input[p] = val
		}
		d.input = append(d.input, input)
		var out float64
		fmt.Sscanf(string(val_strs[len(val_strs)-1]), "%f", &out)
		d.output = append(d.output, out)
	}

	return
}
