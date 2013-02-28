package main

import (
	"flag"
	"fmt"
	"math/rand"
	"time"

	. "github.com/verdverm/go-symexpr"
)

var data_dir = "data/"

var fn = flag.String("data", "F1.data", "data file to analyze")

func main() {
	flag.Parse()
	rand.Seed(time.Now().UnixNano())

	fmt.Println("Hello Gophers\n-----------------")

	srp := defaultParams()
	srch := newSearch(srp)
	srch.initSearch()
	srch.runSearch()

	fmt.Println("Final Results\n-----------------")
	srch.printBestEqns()
}

func defaultParams() *SR_Params {

	var srp SR_Params
	srp.DataFN = data_dir + *fn
	srp.Gens = 100
	srp.Islands = 8

	srp.PopSize = 50
	srp.RptSize = 10
	srp.CrossRate = 0.75
	srp.MutateRate = 0.2

	p := &srp.treep
	p.MaxSize = 50
	p.MinSize = 3
	p.MaxDepth = 6
	p.MinDepth = 1

	p.RootsT = []ExprType{ADD, MUL}
	p.NodesT = []ExprType{VAR, CONSTANTF, ADD, NEG, MUL, DIV, COS, SIN}
	p.NonTrigT = []ExprType{VAR, CONSTANTF, ADD, NEG, MUL, DIV}
	p.LeafsT = []ExprType{VAR, CONSTANTF}

	p.DoSimp = true
	p.SRules = DefaultRules()

	return &srp
}
