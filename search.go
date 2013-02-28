package main

import (
	"fmt"

	expr "github.com/verdverm/go-symexpr"
)

type Eqn struct {
	eqn expr.Expr // embedding the Expr type

	size int
	err  float64
}

func (e *Eqn) String() string {
	return fmt.Sprintf("%d  %.6f    %v\n", e.size, e.err, e.eqn)
}

type EqnChan chan []*Eqn

type SR_Params struct {
	// search parameters
	DataFN string
	treep  TreeParams

	Gens    int
	Islands int

	// island parameters
	PopSize int
	RptSize int

	CrossRate  float64
	MutateRate float64
}

type Search struct {
	// parameters
	params *SR_Params

	// internal data
	data    *DataSet
	isles   []*Island
	perEqns [][]*Eqn

	best []*Eqn

	// internal comm
	reports []EqnChan
}

func newSearch(srp *SR_Params) *Search {
	srch := new(Search)
	srch.params = srp
	return srch
}

func (S *Search) initSearch() {
	fmt.Println("Initializing Search")

	// read data
	S.data = readDataSetFile(S.params.DataFN)

	// set usable vars now that we have data
	S.params.treep.UsableVars = make([]int, S.data.dimensions())
	for d := 0; d < S.data.dimensions(); d++ {
		S.params.treep.UsableVars[d] = d
	}

	// initialize the islands
	S.isles = make([]*Island, S.params.Islands)
	S.perEqns = make([][]*Eqn, S.params.Islands)
	S.reports = make([]EqnChan, S.params.Islands)
	for i := 0; i < S.params.Islands; i++ {
		S.reports[i] = make(EqnChan, 2)
		S.isles[i] = newIsland(i, S.params, S.data, S.reports[i])
		S.isles[i].initIsland()
	}

	S.best = make([]*Eqn, 32)

	fmt.Println("Search Initialized\n")
}

func (S *Search) runSearch() {
	fmt.Println("Running Search\n-------------------\n")

	for g := 0; g < S.params.Gens; g++ {
		fmt.Printf("Gen %3d:\n", g)
		for i := 0; i < S.params.Islands; i++ {
			S.isles[i].step()
		}

		S.recvResults()
	}

	fmt.Println("Maximum Generations Reached\n")

	for i := 0; i < S.params.Islands; i++ {
		S.isles[i].cleanIsland()
	}

}

func (S *Search) recvResults() {
	for i := 0; i < S.params.Islands; i++ {
		S.perEqns[i] = <-S.reports[i]
	}
}

func (S *Search) printBestEqns() {
	temp := make([]*Eqn, 0)
	for i := 0; i < S.params.Islands; i++ {
		temp = append(temp, S.perEqns[i][:]...)
	}

	pareto := NewQueueFromArray(temp)
	pareto.ParetoSort()
	copy(S.best, temp)

	for i := 0; i < len(S.best); i++ {
		fmt.Printf("%d: %v", i, S.best[i])
	}

}

/*
Apprenticeship Patterns - Oreilly Publishing

The Talent Code - Daniel Coyle

Mind, Brain, & Education

The Lean Startup - Eric Ries

*/
