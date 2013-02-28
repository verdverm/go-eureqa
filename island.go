package main

import (
	"fmt"
	"math"
	"math/rand"

	. "github.com/verdverm/go-symexpr"
)

type Island struct {
	// global info
	Id int

	// parameters
	params *SR_Params

	// communication
	report EqnChan

	// internal data
	iters int
	data  *DataSet

	rng *rand.Rand

	eqns []*Eqn // best equations
	offs []*Eqn // offspring equations
}

func newIsland(id int, srp *SR_Params, data *DataSet, rpt EqnChan) *Island {
	isle := new(Island)
	isle.Id = id
	isle.params = srp
	isle.data = data
	isle.report = rpt
	return isle
}

func (I *Island) initIsland() {
	fmt.Println("Initializing Island", I.Id)
	// initialize internal structs
	I.rng = rand.New(rand.NewSource(rand.Int63()))

	// create initial eqns
	I.initEqns()

}

func (I *Island) step() {
	I.evalEqns()
	I.selectEqns()
	I.reportEqns()
	I.breedEqns()
	I.iters++
}

func (I *Island) reportEqns() {

	report := make([]*Eqn, I.params.RptSize)
	copy(report, I.eqns[:I.params.RptSize])
	I.report <- report

}

func (I *Island) evalEqns() {
	for e := 0; e < len(I.eqns); e++ {
		err_sum := 0.0
		for p := 0; p < I.data.length(); p++ {
			f_in := I.data.input[p]
			f_out := I.data.output[p]
			e_out := I.eqns[e].eqn.Eval(0, f_in, nil, nil)
			diff := math.Abs(f_out - e_out)
			err_sum += diff
		}

		I.eqns[e].err = err_sum / float64(I.data.length())
		if badEqnFilter(I.eqns[e]) {
			I.eqns[e] = nil
		}
	}

}

func (I *Island) selectEqns() {

	temp := make([]*Eqn, len(I.eqns))
	copy(temp, I.eqns)
	pareto := NewQueueFromArray(temp)
	pareto.ParetoSort()
	copy(I.eqns, temp)

}

func (I *Island) breedEqns() {
	NE := I.params.PopSize
	for e := 0; e < NE; e++ {

		// select parents with binary tournament
		rnum1, rnum2, rnum3, rnum4 := I.rng.Intn(NE), I.rng.Intn(NE), I.rng.Intn(NE), I.rng.Intn(NE)
		if rnum3 < rnum1 {
			rnum1 = rnum3
		}
		if rnum4 < rnum2 {
			rnum2 = rnum4
		}
		if I.eqns[rnum1] == nil || I.eqns[rnum2] == nil {
			e--
			continue
		}
		p1 := I.eqns[rnum1].eqn
		p2 := I.eqns[rnum2].eqn
		if p1 == nil || p2 == nil {
			e--
			continue
		}

		// production of one child from two parents
		var eqnSimp Expr
		for {
			var new_eqn Expr

			// cross equations
			if I.rng.Float64() < I.params.CrossRate {
				new_eqn = CrossEqns_Vanilla(p1, p2, &I.params.treep, I.rng)
			} else {
				new_eqn = InjectEqn_Vanilla(p1, &I.params.treep, I.rng)
			}

			// mutate equation
			if I.rng.Float64() < I.params.MutateRate {
				MutateEqn_Vanilla(new_eqn, &I.params.treep, I.rng)
			}

			// simplify equation
			eqnSimp = new_eqn.Simplify(I.params.treep.SRules)
			if eqnSimp == nil || !(eqnSimp.HasVar()) {
				continue
			}
			eqnSimp.CalcExprStats()

			I.params.treep.ResetCurr()
			I.params.treep.ResetTemp()
			if I.params.treep.CheckExpr(eqnSimp) {
				break
			}
		}
		I.offs[e] = &Eqn{eqnSimp, eqnSimp.Size(), -1.0}
	}
}

func (I *Island) initEqns() {
	I.eqns = make([]*Eqn, I.params.PopSize)
	I.offs = make([]*Eqn, I.params.PopSize)
	for e := 0; e < len(I.eqns); e++ {
		new_eqn := ExprGen(&I.params.treep, I.rng)
		// fmt.Printf("%d: %v\n", e, new_eqn)
		I.eqns[e] = &Eqn{new_eqn, new_eqn.Size(), -1.0} // -1 because actual errors are >= 0
	}

}

func (I *Island) cleanIsland() {
	fmt.Println("Isle ", I.Id)
	for e := 0; e < 10; e++ {
		fmt.Print(I.eqns[e])
	}
	fmt.Println()

}
