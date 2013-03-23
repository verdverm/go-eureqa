package gpsr

import (
	"math/rand"

	. "damd/go-symexpr"
	prob "damd/problems"
)

func ExprGen(egp *prob.TreeParams, srules SimpRules, rng *rand.Rand) Expr {
	var ret Expr
	good := false
	cnt := 0

	for !good {
		egp.ResetCurr()
		egp.ResetTemp()

		//     eqn := ExprGenStats( &egp, rng )
		eqn := exprGrow(-1, ExprGenDepth, egp, rng)
		eqn.CalcExprStats(0)

		//     fmt.Printf( "%v\n", eqn)

		ret = eqn.Simplify(srules)
		ret.CalcExprStats(0)

		//     fmt.Printf( "%v\n\n", ret)

		// check eqn after simp
		good = egp.CheckExpr(ret)
		cnt++
	}
	return ret
}

type ExprGenFunc (func(egp *prob.TreeParams, rng *rand.Rand) Expr)

func exprGrow(e ExprType, egfunc ExprGenFunc, egp *prob.TreeParams, rng *rand.Rand) Expr {

	if e == -1 {
		return egfunc(egp, rng)
	}

	switch {

	case e == TIME:
		return NewTime()
	case e == VAR:
		p := egp.UsableVars[rng.Intn(len(egp.UsableVars))]
		return NewVar(p)
	case e == CONSTANT:
		i := egp.CoeffCount
		egp.CoeffCount++
		return NewConstant(i)
	case e == CONSTANTF:
		return NewConstantF(rng.NormFloat64() * 2.0)
	case e == SYSTEM:
		return NewSystem(rng.Intn(egp.NumSys))

	case e == NEG:
		egp.CurrDepth++
		return NewNeg(egfunc(egp, rng))
	case e == ABS:
		egp.CurrDepth++
		return NewAbs(egfunc(egp, rng))
	case e == SQRT:
		egp.CurrDepth++
		return NewSqrt(egfunc(egp, rng))
	case e == SIN:
		egp.CurrDepth++
		egp.InTrig = true
		tmp := NewSin(egfunc(egp, rng))
		egp.InTrig = false
		return tmp
	case e == COS:
		egp.CurrDepth++
		egp.InTrig = true
		tmp := NewCos(egfunc(egp, rng))
		egp.InTrig = false
		return tmp
	case e == TAN:
		egp.CurrDepth++
		egp.InTrig = true
		tmp := NewTan(egfunc(egp, rng))
		egp.InTrig = false
		return tmp
	case e == EXP:
		egp.CurrDepth++
		return NewExp(egfunc(egp, rng))
	case e == LOG:
		egp.CurrDepth++
		return NewLog(egfunc(egp, rng))

	case e == POWI:
		egp.CurrDepth++
		return NewPowI(egfunc(egp, rng), (rng.Int()%7)-3)
	case e == POWF:
		egp.CurrDepth++
		return NewPowF(egfunc(egp, rng), rng.Float64()*3.0)
	case e == POWE:
		egp.CurrDepth++
		return NewPowE(egfunc(egp, rng), egfunc(egp, rng))

	case e == ADD:
		egp.CurrDepth++
		add := NewAdd()
		add.Insert(egfunc(egp, rng))
		add.Insert(egfunc(egp, rng))
		return add
	case e == MUL:
		egp.CurrDepth++
		mul := NewMul()
		mul.Insert(egfunc(egp, rng))
		mul.Insert(egfunc(egp, rng))
		return mul
	case e == DIV:
		egp.CurrDepth++
		return NewDiv(egfunc(egp, rng), egfunc(egp, rng))
	}
	return NewNull()
}

func ExprGenDepth(egp *prob.TreeParams, rng *rand.Rand) Expr {
	rnum := rng.Int()
	var e ExprType

	if egp.CurrDepth == 0 {
		e = egp.RootsT[rnum%len(egp.RootsT)]
	} else if egp.CurrDepth >= egp.TmpMaxDepth {
		e = egp.LeafsT[rnum%len(egp.LeafsT)]
	} else {
		if egp.InTrig {
			e = egp.NonTrigT[rnum%len(egp.NonTrigT)]
		} else {
			e = egp.NodesT[rnum%len(egp.NodesT)] // this is to deal with NULL (ie so we dont get switch on 0)
		}
	}

	return exprGrow(e, ExprGenDepth, egp, rng)
}

func ExprGenSize(egp *prob.TreeParams, rng *rand.Rand) Expr {
	rnum := rng.Int()
	var e ExprType

	if egp.CurrDepth == 0 {
		e = egp.RootsT[rnum%len(egp.RootsT)]
	} else if egp.CurrDepth >= egp.MaxDepth {
		e = egp.LeafsT[rnum%len(egp.LeafsT)]
	} else {
		if egp.InTrig {
			e = egp.NonTrigT[rnum%len(egp.NonTrigT)]
		} else {
			e = egp.NodesT[rnum%len(egp.NodesT)] // this is to deal with NULL (ie so we dont get switch on 0)
		}
	}

	return exprGrow(e, ExprGenSize, egp, rng)
}

func CrossEqns_Vanilla(p1, p2 Expr, egp *prob.TreeParams, rng *rand.Rand) Expr {
	eqn := p1.Clone()
	eqn.CalcExprStats(0)

	s1, s2 := rng.Intn(eqn.Size()), rng.Intn(p2.Size())
	//   eqn.SetExpr(&s1, p2.GetExpr(&s2).Clone())
	//   eqn.SetExpr(&s1, new_eqn )
	SwapExpr(eqn, p2.GetExpr(&s2).Clone(), s1)

	eqn.CalcExprStats(0)
	return eqn

}

func InjectEqn_Vanilla(p1 Expr, egp *prob.TreeParams, rng *rand.Rand) Expr {
	eqn := p1.Clone()
	eqn.CalcExprStats(0)

	s1 := rng.Intn(eqn.Size())
	s2 := s1
	e2 := eqn.GetExpr(&s2)

	egp.CurrSize = eqn.Size() - e2.Size()
	egp.CurrDepth = e2.Depth()
	egp.ResetTemp()

	// not correct (should be size based)
	new_eqn := exprGrow(-1, ExprGenDepth, egp, rng)
	//   eqn.SetExpr(&s1, new_eqn )
	SwapExpr(eqn, new_eqn, s1)

	eqn.CalcExprStats(0)
	return eqn
}

func InjectEqn_50_150(p1 Expr, egp *prob.TreeParams, rng *rand.Rand) Expr {
	eqn := p1.Clone()
	eqn.CalcExprStats(0)

	// begin loop
	s1 := rng.Intn(eqn.Size())
	s1_tmp := s1
	e1 := eqn.GetExpr(&s1_tmp)

	egp.ResetCurr()
	egp.ResetTemp()
	egp.TmpMinSize = e1.Size() / 2
	egp.TmpMaxSize = (e1.Size() * 3) / 2
	// loop if min/max out of bounds
	// and select new subtree

	// not correct (should be size based)
	new_eqn := exprGrow(-1, ExprGenDepth, egp, rng)
	//   eqn.SetExpr(&s1, new_eqn )
	SwapExpr(eqn, new_eqn, s1)
	eqn.CalcExprStats(0)
	return eqn
}

func InjectEqn_SubtreeFair(p1 Expr, egp *prob.TreeParams, rng *rand.Rand) Expr {
	eqn := p1.Clone()
	eqn.CalcExprStats(0)

	// begin loop
	s1, s2 := rng.Intn(eqn.Size()), rng.Intn(eqn.Size())
	s2_tmp := s2
	e2 := eqn.GetExpr(&s2_tmp)

	egp.ResetCurr()
	egp.ResetTemp()
	egp.TmpMinSize = e2.Size() / 2
	egp.TmpMaxSize = (e2.Size() * 3) / 2
	// loop if min/max out of bounds
	// and select new subtree

	// not correct (should be size based)
	new_eqn := exprGrow(-1, ExprGenDepth, egp, rng)
	//   eqn.SetExpr(&s1, new_eqn )
	SwapExpr(eqn, new_eqn, s1)

	eqn.CalcExprStats(0)
	return eqn
}

func MutateEqn_Vanilla(eqn Expr, egp *prob.TreeParams, rng *rand.Rand, sysvals []float64) {
	mut := false
	for !mut {
		s2 := rng.Intn(eqn.Size())
		s2_tmp := s2
		e2 := eqn.GetExpr(&s2_tmp)

		t := e2.ExprType()

		switch t {
		case CONSTANTF:
			if egp.NumSys == 0 {
				e2.(*ConstantF).F += rng.NormFloat64()
			} else {
				// mod coeff
				if rng.Intn(2) == 0 {
					e2.(*ConstantF).F += rng.NormFloat64()
				} else {
					// coeff -> system
					var mul Mul
					s := &System{P: rng.Intn(egp.NumSys)}
					e2.(*ConstantF).F /= sysvals[s.P]
					mul.CS[0] = s
					mul.CS[1] = e2.(*ConstantF)
					e2 = &mul
				}
			}
			mut = true
		case SYSTEM:
			e2.(*System).P = rng.Int() % egp.NumSys
			mut = true
		case VAR:
			p := egp.UsableVars[rng.Intn(len(egp.UsableVars))]
			e2.(*Var).P = p

			// r := rng.Intn(len(egp.UsableVars))
			// e2.(*Var).P = egp.UsableVars[r]
			mut = true
		case ADD:

		case MUL:

		}
	}
}
