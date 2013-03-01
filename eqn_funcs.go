package main

import (
	"math"
	"math/rand"
	"sort"

	. "github.com/verdverm/go-symexpr"
)

type TreeParams struct {

	// bounds on tree
	MaxSize, MaxDepth,
	MinSize, MinDepth int

	// usable terms at each location
	RootsT, NodesT, LeafsT, NonTrigT []ExprType

	// simplify options
	DoSimp bool
	SRules SimpRules

	// bounds on some operands
	UsableVars               []int
	NumDim, NumSys, NumCoeff int

	// tpm bounds on tree (for subtree distributions)
	TmpMaxSize, TmpMaxDepth,
	TmpMinSize, TmpMinDepth int

	// Current values
	CurrSize, CurrDepth int
	InTrig              bool
	CoeffCount          int
}

func ExprGen(egp *TreeParams, rng *rand.Rand) Expr {
	var ret Expr
	good := false
	cnt := 0

	for !good {
		egp.ResetCurr()
		egp.ResetTemp()

		//     eqn := ExprGenStats( &egp, rng )
		eqn := exprGrow(-1, ExprGenDepth, egp, rng)
		eqn.CalcExprStats()

		//     fmt.Printf( "%v\n", eqn)
		eqnSimp := eqn.Simplify(egp.SRules)
		if eqnSimp == nil || !(eqnSimp.HasVar()) {
			continue
		}
		eqnSimp.CalcExprStats()
		//     fmt.Printf( "%v\n\n", ret)

		// check eqn after simp
		good = egp.CheckExpr(eqnSimp)
		if good {
			ret = eqnSimp
		}
		cnt++
	}
	return ret
}

type ExprGenFunc (func(egp *TreeParams, rng *rand.Rand) Expr)

func exprGrow(e ExprType, egfunc ExprGenFunc, egp *TreeParams, rng *rand.Rand) Expr {

	if e == -1 {
		return egfunc(egp, rng)
	}

	switch {

	case e == VAR:
		p := egp.UsableVars[rng.Intn(len(egp.UsableVars))]
		return NewVar(p)
	case e == CONSTANTF:
		return NewConstantF(rng.NormFloat64() * 2.0)

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

func ExprGenDepth(egp *TreeParams, rng *rand.Rand) Expr {
	rnum := rng.Int()
	var e ExprType

	if egp.CurrDepth >= egp.TmpMaxDepth {
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

func CrossEqns_Vanilla(p1, p2 Expr, egp *TreeParams, rng *rand.Rand) Expr {
	eqn := p1.Clone()
	eqn.CalcExprStats()

	s1, s2 := rng.Intn(eqn.Size()), rng.Intn(p2.Size())
	//   eqn.SetExpr(&s1, p2.GetExpr(&s2).Clone())
	//   eqn.SetExpr(&s1, new_eqn )
	SwapExpr(eqn, p2.GetExpr(&s2).Clone(), s1)

	eqn.CalcExprStats()
	return eqn

}

func InjectEqn_Vanilla(p1 Expr, egp *TreeParams, rng *rand.Rand) Expr {
	eqn := p1.Clone()
	eqn.CalcExprStats()

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

	eqn.CalcExprStats()
	return eqn
}

func MutateEqn_Vanilla(eqn Expr, egp *TreeParams, rng *rand.Rand) {
	mut := false
	for !mut {
		s2 := rng.Intn(eqn.Size())
		s2_tmp := s2
		e2 := eqn.GetExpr(&s2_tmp)

		t := e2.ExprType()

		switch t {
		case CONSTANTF:
			e2.(*ConstantF).F += rng.NormFloat64()
			mut = true
		case VAR:
			p := egp.UsableVars[rng.Intn(len(egp.UsableVars))]
			e2.(*Var).P = p
			mut = true
		case ADD:

		case MUL:

		}
	}
}

type EqnListNode struct {
	Rpt      *Eqn
	prv, nxt *EqnListNode
}

func (self *EqnListNode) Next() *EqnListNode {
	return self.nxt
}
func (self *EqnListNode) Prev() *EqnListNode {
	return self.prv
}

type EqnList struct {
	length int
	Head   *EqnListNode
	Tail   *EqnListNode
}

func (self *EqnList) Len() int {
	return self.length
}

func (self *EqnList) Front() (node *EqnListNode) {
	return self.Head
}
func (self *EqnList) Back() (node *EqnListNode) {
	return self.Tail
}

func (self *EqnList) PushFront(rpt *Eqn) {
	tmp := new(EqnListNode)
	tmp.Rpt = rpt
	tmp.nxt = self.Head
	if self.Head != nil {
		self.Head.prv = tmp
	}
	self.Head = tmp
	if self.Tail == nil {
		self.Tail = tmp
	}
	self.length++
}
func (self *EqnList) PushBack(rpt *Eqn) {
	tmp := new(EqnListNode)
	tmp.Rpt = rpt
	tmp.prv = self.Tail
	if self.Tail != nil {
		self.Tail.nxt = tmp
	}
	self.Tail = tmp
	if self.Head == nil {
		self.Head = tmp
	}
	self.length++
}
func (self *EqnList) Remove(node *EqnListNode) {
	if node.prv != nil {
		node.prv.nxt = node.nxt
	} else {
		self.Head = node.nxt
	}
	if node.nxt != nil {
		node.nxt.prv = node.prv
	} else {
		self.Tail = node.prv
	}
	node.prv = nil
	node.nxt = nil
	self.length--
}

type EqnArray []*Eqn

func (ea EqnArray) Len() int { return len(ea) }
func (ea EqnArray) Less(i, j int) bool {
	if ea[i] == nil {
		return false
	}
	if ea[j] == nil {
		return true
	}
	return ea[i].eqn.AmILess(ea[j].eqn)
}
func (ea EqnArray) Swap(i, j int) {
	ea[i], ea[j] = ea[j], ea[i]
}

type SortType int

const (
	NULL_SORT SortType = iota
	PARETO_SORT
	EQN_SORT
)

// NOTE!!!  the storage is reverse order
type EqnQueue struct {
	queue      []*Eqn
	less       func(i, j *Eqn) bool
	sortmethod SortType
}

func NewQueueFromArray(era []*Eqn) *EqnQueue {
	Q := new(EqnQueue)
	Q.queue = era
	return Q
}

func NewEqnQueue() *EqnQueue {
	B := new(EqnQueue)
	B.queue = make([]*Eqn, 0) // zero means empty
	return B
}

func (bb EqnQueue) Len() int { return len(bb.queue) }
func (bb EqnQueue) Less(i, j int) bool {
	if bb.queue[i] == nil {
		return false
	}
	if bb.queue[j] == nil {
		return true
	}
	return bb.less(bb.queue[i], bb.queue[j])
}
func (bb EqnQueue) Swap(i, j int) {
	bb.queue[i], bb.queue[j] = bb.queue[j], bb.queue[i]
}

func lessSizeError(l, r *Eqn) bool {
	sz := l.size - r.size
	if sz < 0 {
		return true
	} else if sz > 0 {
		return false
	}
	sc := l.err - r.err
	if sc < 0.0 {
		return true
	} else if sc > 0.0 {
		return false
	}
	return l.eqn.AmILess(r.eqn)
}

func (bb *EqnQueue) ParetoSort() {
	bb.less = lessSizeError
	sort.Sort(bb)

	// var pareto list.List
	// pareto.Init()
	var pareto EqnList
	for i, _ := range bb.queue {
		if bb.queue[i] == nil {
			continue
		}
		pareto.PushBack(bb.queue[i])
	}

	over := len(bb.queue) - 1
	for pareto.Len() > 0 && over >= 0 {
		pe := pareto.Front()
		eLast := pe
		pb := pe.Rpt
		cSize := pb.size
		cScore := pb.err
		pe = pe.Next()
		for pe != nil && over >= 0 {
			pb := pe.Rpt
			sz := pb.size
			if sz > cSize {
				cSize = sz
				if pb.err < cScore {
					cScore = pb.err
					bb.queue[over] = eLast.Rpt
					over--
					pareto.Remove(eLast)
					eLast = pe
				}
			}
			pe = pe.Next()
		}
		if over < 0 {
			break
		}

		bb.queue[over] = eLast.Rpt
		over--
		pareto.Remove(eLast)
	}

	// reverse the queue
	i, j := 0, len(bb.queue)-1
	for i < j {
		bb.Swap(i, j)
		i++
		j--
	}
}

func (tp *TreeParams) CheckExpr(e Expr) bool {
	if e.Size() < tp.MinSize {
		//    fmt.Printf( "Too SMALL:  e:%v  l:%v\n", e.Size(), tp.TmpMinSize )
		return false
	} else if e.Size() > tp.MaxSize {
		//    fmt.Printf( "Too LARGE:  e:%v  l:%v\n", e.Size(), tp.TmpMaxSize )
		return false
	} else if e.Height() < tp.MinDepth {
		//    fmt.Printf( "Too SHORT:  e:%v  l:%v\n", e.Height(), tp.TmpMinDepth )
		return false
	} else if e.Height() > tp.MaxDepth {
		//    fmt.Printf( "Too TALL:  e:%v  l:%v\n", e.Height(), tp.TmpMaxDepth )
		return false
	}
	return true
}

func (tp *TreeParams) ResetCurr() {
	tp.CurrSize, tp.CurrDepth, tp.InTrig, tp.CoeffCount = 0, 0, false, 0
}
func (tp *TreeParams) ResetTemp() {
	tp.TmpMaxSize, tp.TmpMaxDepth = tp.MaxSize, tp.MaxDepth
	tp.TmpMinSize, tp.TmpMinDepth = tp.MinSize, tp.MinDepth
}

func (t *TreeParams) Clone() *TreeParams {
	n := new(TreeParams)
	n.MaxSize = t.MaxSize
	n.MaxDepth = t.MaxDepth
	n.MinSize = t.MinSize
	n.MinDepth = t.MinDepth

	n.RootsT = make([]ExprType, len(t.RootsT))
	copy(n.RootsT, t.RootsT)
	n.NodesT = make([]ExprType, len(t.NodesT))
	copy(n.NodesT, t.NodesT)
	n.LeafsT = make([]ExprType, len(t.LeafsT))
	copy(n.LeafsT, t.LeafsT)
	n.NonTrigT = make([]ExprType, len(t.NonTrigT))
	copy(n.NonTrigT, t.NonTrigT)

	n.DoSimp = t.DoSimp
	n.SRules = t.SRules

	n.UsableVars = make([]int, len(t.UsableVars))
	copy(n.UsableVars, t.UsableVars)

	n.NumDim = t.NumDim
	n.NumSys = t.NumSys
	n.NumCoeff = t.NumCoeff

	return n
}

func badEqnFilter(eqn *Eqn) bool {
	if eqn.err > 1e9 ||
		math.IsInf(eqn.err, 0) ||
		math.IsNaN(eqn.err) {
		return true
	}
	return false
}
