package gpsr

import (
	"bufio"
	"fmt"
	"log"
	rand "math/rand"
	"os"

	expr "damd/go-symexpr"
	probs "damd/problems"
)

type EqnIsland struct {
	id   int
	gen  int
	rng  *rand.Rand
	stop bool

	// communitactions stuff
	eqnCmd    chan int
	eqnRpt    chan *probs.ExprReportArray
	eqnInMig  chan *probs.ExprReportArray
	eqnOutMig []chan *probs.ExprReportArray
	eqnGen    chan [2]int // id && gen
	ssetRcv   chan []*probs.PntSubset

	// common config options
	numEqns    int
	eqnBroodSz int
	crossRate  float64
	mutateRate float64

	eqnMigEpoch int
	eqnMigCount int
	eqnRptEpoch int
	eqnRptCount int

	// extra config options
	treecfg *probs.TreeParams
	trie    *IpreNode

	// externally supplied
	prob  *probs.ExprProblem
	ssets []*probs.PntSubset

	// internal data
	// -------------
	parents  probs.ExprReportArray
	brood    []probs.ExprReportArray
	pareto   probs.ExprReportArray
	migrants probs.ExprReportArray
	custom   []probs.ExprReportArray

	// statistics
	neqns    int
	minError float64

	// logs
	logDir        string
	mainLog       *log.Logger
	mainLogBuf    *bufio.Writer
	eqnsLog       *log.Logger
	eqnsLogBuf    *bufio.Writer
	ssetLog       *log.Logger
	ssetLogBuf    *bufio.Writer
	errLog        *log.Logger
	errLogBuf     *bufio.Writer
	fitnessLog    *log.Logger
	fitnessLogBuf *bufio.Writer
	ipreLog       *log.Logger
	ipreLogBuf    *bufio.Writer
}

func (isle *EqnIsland) copyInConfig(gs *GpsrSearch) {
	gp := gs.cnfg

	isle.prob = gs.prob
	isle.treecfg = gp.treecfg.Clone()
	isle.numEqns = gp.numEqns
	isle.eqnBroodSz = gp.eqnBroodSz
	isle.eqnMigEpoch = gp.eqnMigEpoch
	isle.eqnMigCount = gp.eqnMigCount
	isle.eqnRptEpoch = gp.eqnRptEpoch
	isle.eqnRptCount = gp.eqnRptCount
	isle.crossRate = gp.eqnCrossRate
	isle.mutateRate = gp.eqnMutateRate

	isle.eqnCmd = gs.eqnCmd[isle.id]
	isle.eqnRpt = gs.eqnRpt[isle.id]
	isle.eqnInMig = gs.eqnMig[isle.id]
	if gp.numEqnIsles > 1 {
		isle.eqnOutMig = make([]chan *probs.ExprReportArray, 2)
		m1 := ((isle.id + gp.numEqnIsles - 1) % gp.numEqnIsles)
		p1 := ((isle.id + 1) % gp.numEqnIsles)
		isle.eqnOutMig[0] = gs.eqnMig[m1]
		isle.eqnOutMig[1] = gs.eqnMig[p1]
	}
	isle.eqnGen = gs.eqnGen
	isle.ssetRcv = gs.ssetPub[isle.id]

	isle.logDir = gs.logDir + fmt.Sprintf("eisle%d/", isle.id)
}

func (isle *EqnIsland) init() {
	fmt.Println("Initializing EqnIsland ", isle.id)

	// create random number generator
	isle.rng = rand.New(rand.NewSource(rand.Int63()))

	// init logs
	isle.initLogs()
	isle.mainLog.Println("Initializing EqnIsland ", isle.id)

	isle.minError = 1000000.0

	isle.trie = new(IpreNode)
	isle.trie.val = -1
	isle.trie.next = make(map[int]*IpreNode)

	// create random equations
	isle.initEqns()

	// evaluate initial equations
	isle.initEval()

	// select within brood to pareto
	isle.selectBroodToPareto(probs.GPSORT_TRN_ERR)
	isle.eqnsLog.Println("EqnIsle Init Best of Brood")
	for i := 0; i < isle.numEqns; i++ {
		isle.eqnsLog.Println(isle.pareto[i])
	}
	isle.eqnsLog.Println()
	isle.eqnsLog.Println()

	// sort pareto
	queue := probs.NewQueueFromArray(isle.pareto)
	queue.SetSort(probs.GPSORT_PARETO_TRN_ERR)
	queue.Sort()

	isle.eqnsLog.Println("EqnIsle Init Pareto Sorted")
	for i := 0; i < isle.numEqns; i++ {
		isle.eqnsLog.Println(isle.pareto[i])
	}
	isle.eqnsLog.Println()
	isle.eqnsLog.Println()

	// select for parents (just a copy from pareto to parents)
	for i := 0; i < isle.numEqns; i++ {
		isle.parents[i] = isle.pareto[i]
	}

	// report best eqns
	isle.report()
	// rpt := make(probs.ExprReportArray, isle.eqnRptCount)
	// for i := 0; i < isle.eqnRptCount; i++ {
	// 	// fmt.Println(i)
	// 	rpt[i] = isle.parents[i].Clone()
	// }
	// isle.eqnRpt <- &rpt

	isle.breed()

	// flush logs at end of init
	isle.flushLogs()

}

func (isle *EqnIsland) run() {
	fmt.Println("Running EqnIsland ", isle.id)

	isle.messages()
	for !isle.stop {
		isle.step()
		isle.messages()
	}
	fmt.Println("EqnIsle", isle.id, " exiting")
	isle.eqnCmd <- -1
}

func (isle *EqnIsland) clean() {
	isle.mainLog.Println("Cleaning EqnIsland ", isle.id)

	isle.flushLogs()
}

func (isle *EqnIsland) step() {

	isle.eval()
	isle.selecting()

	isle.gen++

	isle.report()
	isle.migrate()
	isle.breed()

	isle.eqnGen <- [2]int{isle.id, isle.gen}
	isle.flushLogs()

}

func (isle *EqnIsland) messages() {
	// isle.mainLog.Println("Messages EqnIsland ", isle.id, isle.gen)

	for {
		select {
		// check upstream messages 
		case cmd, ok := <-isle.eqnCmd:
			if ok {
				switch cmd {
				case -1:
					// end processing
					fmt.Println("EqnIsle", isle.id, " Stopping")
					isle.stop = true
					return
				}
			}

		// check for immigrants
		case migs, ok := <-isle.eqnInMig:
			if ok {
				isle.mainLog.Println("new isle.migrants ", isle.id, isle.gen)
				isle.migrants = append(isle.migrants, (*migs)[:]...)
			}

		case ssets, ok := <-isle.ssetRcv:
			if ok {
				isle.mainLog.Println("new isle.ssets ", isle.id, isle.gen)
				isle.ssets = ssets
			}

		// so we don't wait indefinitely for msg in select
		default:
			return
		}
	}
}
func (isle *EqnIsland) eval() {
	// isle.mainLog.Println("Evaluating EqnIsland ", isle.id, isle.gen)

	for i := 0; i < isle.numEqns; i++ {
		calcEqnPredErr(isle.brood[i], isle.ssets, isle.prob)
		for j, e := range isle.brood[i] {
			if badEqnFilterPred(e) {
				isle.brood[i][j] = nil
			}
		}
	}

}
func (isle *EqnIsland) selecting() {
	// isle.mainLog.Println("Selecting EqnIsland ", isle.id, isle.gen)

	// select within brood to pareto
	isle.selectBroodToPareto(probs.GPSORT_PRE_ERR)

	// add migrants to pareto (may need to increase size)
	pLen := isle.pareto.Len()
	isle.pareto = append(isle.pareto, isle.migrants...)

	// sort pareto
	queue := probs.NewQueueFromArray(isle.pareto)
	queue.SetSort(probs.GPSORT_PARETO_PRE_ERR)
	queue.Sort()

	// select for parents (just a copy from pareto to parents)
	for i := 0; i < isle.numEqns; i++ {
		isle.parents[i] = isle.pareto[i]
	}

	isle.pareto = isle.pareto[:pLen]

}
func (isle *EqnIsland) report() {

	rpt := make(probs.ExprReportArray, isle.eqnRptCount)
	errSum, errCnt := 0.0, 0
	p := 0
	for i := 0; i < len(isle.parents) && p < isle.eqnRptCount; i++ {
		if isle.parents[i] == nil {
			continue
		}
		r := isle.parents[i].Clone()
		if r != nil && r.Expr() != nil {
			if r.PredError() < isle.minError {
				isle.minError = r.PredError()
			}
			errSum += r.PredError()
			errCnt++
		}
		rpt[p] = r
		p++
	}
	// isle.ipreLog.Println(isle.gen, isle.neqns, isle.trie.cnt, isle.trie.vst)
	isle.fitnessLog.Println(isle.gen, isle.neqns, isle.trie.cnt, isle.trie.vst, errSum/float64(errCnt), isle.minError)

	// send up best eqns from parents  (archive eventually)
	if isle.gen%isle.eqnRptEpoch == 0 {
		isle.mainLog.Println("Reporting EqnIsland ", isle.id, isle.gen)
		isle.eqnsLog.Println(rpt)
		isle.eqnRpt <- &rpt
	}
}

func (isle *EqnIsland) migrate() {

	// send migrants
	if isle.gen%isle.eqnMigEpoch == 0 && len(isle.eqnOutMig) > 0 {
		// isle.mainLog.Println("Migrating EqnIsland ", isle.id, isle.gen)
		mig1 := make(probs.ExprReportArray, isle.eqnMigCount)
		mig2 := make(probs.ExprReportArray, isle.eqnMigCount)
		for i := 0; i < isle.eqnMigCount; i++ {
			mig1[i] = isle.parents[i].Clone()
			mig2[i] = isle.parents[i].Clone()
		}
		isle.eqnOutMig[0] <- &mig1
		isle.eqnOutMig[1] <- &mig2
	}
}
func (isle *EqnIsland) breed() {
	// isle.mainLog.Println("Breeding EqnIsland ", isle.id, isle.gen)

	NE := isle.numEqns
	for e := 0; e < NE; e++ {

		// select parents for brood production
		rnum1, rnum2, rnum3, rnum4 := isle.rng.Intn(NE), isle.rng.Intn(NE), isle.rng.Intn(NE), isle.rng.Intn(NE)
		if rnum3 < rnum1 {
			rnum1 = rnum3
		}
		if rnum4 < rnum2 {
			rnum2 = rnum4
		}
		if isle.parents[rnum1] == nil || isle.parents[rnum2] == nil {
			e--
			continue
		}
		p1 := isle.parents[rnum1].Expr()
		p2 := isle.parents[rnum2].Expr()
		if p1 == nil || p2 == nil {
			e--
			continue
		}

		for b := 0; b < isle.eqnBroodSz; b++ {

			// production of one child from two parents
			var eqnSimp expr.Expr
			for {
				var new_eqn expr.Expr

				// cross equations
				if isle.rng.Float64() < isle.crossRate {
					new_eqn = CrossEqns_Vanilla(p1, p2, isle.treecfg, isle.rng)
				} else {
					new_eqn = InjectEqn_SubtreeFair(p1, isle.treecfg, isle.rng)
				}

				// mutate equation
				if isle.rng.Float64() < isle.mutateRate {
					MutateEqn_Vanilla(new_eqn, isle.treecfg, isle.rng, nil)
				}

				// simplify equation
				eqnSimp = new_eqn.Simplify(isle.treecfg.SRules)
				if eqnSimp == nil || !(eqnSimp.HasVar()) {
					continue
				}
				eqnSimp.CalcExprStats(0)

				isle.treecfg.ResetCurr()
				isle.treecfg.ResetTemp()
				if isle.treecfg.CheckExpr(eqnSimp) {
					break
				}
			}

			isle.neqns++
			serial := make([]int, 0, 64)
			serial = eqnSimp.Serial(serial)
			isle.trie.InsertSerial(serial)

			isle.brood[e][b] = new(probs.ExprReport)
			isle.brood[e][b].SetExpr(eqnSimp)
			isle.brood[e][b].SetProcID(isle.id)
			isle.brood[e][b].SetIterID(isle.gen)
			uid := isle.gen*(isle.numEqns*isle.eqnBroodSz) + e*(isle.eqnBroodSz) + b
			isle.brood[e][b].SetUnitID(uid)

		}

	}

}

func (isle *EqnIsland) flushLogs() {
	isle.errLogBuf.Flush()
	isle.mainLogBuf.Flush()
	isle.ssetLogBuf.Flush()
	isle.eqnsLogBuf.Flush()
	isle.fitnessLogBuf.Flush()
	isle.ipreLogBuf.Flush()
}

func (isle *EqnIsland) initLogs() {
	os.Mkdir(isle.logDir, os.ModePerm)
	tmpF0, err5 := os.Create(isle.logDir + fmt.Sprintf("eisle%d:err.log", isle.id))
	if err5 != nil {
		log.Fatal("couldn't create errs log")
	}
	isle.errLogBuf = bufio.NewWriter(tmpF0)
	isle.errLog = log.New(isle.errLogBuf, "", log.LstdFlags)

	tmpF1, err1 := os.Create(isle.logDir + fmt.Sprintf("eisle%d:main.log", isle.id))
	if err1 != nil {
		log.Fatal("couldn't create main log")
	}
	isle.mainLogBuf = bufio.NewWriter(tmpF1)
	isle.mainLog = log.New(isle.mainLogBuf, "", log.LstdFlags)

	tmpF2, err2 := os.Create(isle.logDir + fmt.Sprintf("eisle%d:eqns.log", isle.id))
	if err2 != nil {
		log.Fatal("couldn't create eqns log")
	}
	isle.eqnsLogBuf = bufio.NewWriter(tmpF2)
	isle.eqnsLog = log.New(isle.eqnsLogBuf, "", log.LstdFlags)

	tmpF3, err3 := os.Create(isle.logDir + fmt.Sprintf("eisle%d:sset.log", isle.id))
	if err3 != nil {
		log.Fatal("couldn't create ssets log")
	}
	isle.ssetLogBuf = bufio.NewWriter(tmpF3)
	isle.ssetLog = log.New(isle.ssetLogBuf, "", log.LstdFlags)

	tmpF5, err5 := os.Create(isle.logDir + fmt.Sprintf("eisle%d:fitness.log", isle.id))
	if err5 != nil {
		log.Fatal("couldn't create eqns log")
	}
	isle.fitnessLogBuf = bufio.NewWriter(tmpF5)
	isle.fitnessLogBuf.Flush()
	isle.fitnessLog = log.New(isle.fitnessLogBuf, "", log.Ltime|log.Lmicroseconds)

	tmpF4, err4 := os.Create(isle.logDir + fmt.Sprintf("eisle%d:ipre.log", isle.id))
	if err4 != nil {
		log.Fatal("couldn't create eqns log")
	}
	isle.ipreLogBuf = bufio.NewWriter(tmpF4)
	isle.ipreLogBuf.Flush()
	isle.ipreLog = log.New(isle.ipreLogBuf, "", log.Ltime|log.Lmicroseconds)
}

func (isle *EqnIsland) initEqns() {
	// initialize empty parents & pareto
	isle.parents = make(probs.ExprReportArray, isle.numEqns)
	isle.pareto = make(probs.ExprReportArray, isle.numEqns, isle.numEqns*2)

	// initialize new Exprs into brood
	srules := isle.treecfg.SRules
	fmt.Printf("srules: %v\n", srules)
	isle.brood = make([]probs.ExprReportArray, isle.numEqns)
	for i := 0; i < isle.numEqns; i++ {
		isle.brood[i] = make(probs.ExprReportArray, isle.eqnBroodSz)
		isle.eqnsLog.Println("EqnIsle Init Brood ", i)
		for j := 0; j < isle.eqnBroodSz; j++ {
			var e expr.Expr
			for {
				new_eqn := ExprGen(isle.treecfg, srules, isle.rng)

				// simplify equation
				eqnSimp := new_eqn.Simplify(isle.treecfg.SRules)
				if eqnSimp == nil || !(eqnSimp.HasVar()) {
					continue
				}
				eqnSimp.CalcExprStats(0)

				isle.treecfg.ResetCurr()
				isle.treecfg.ResetTemp()
				if isle.treecfg.CheckExpr(eqnSimp) {
					e = eqnSimp
					break
				}
			}

			isle.neqns++
			serial := make([]int, 0, 64)
			serial = e.Serial(serial)
			isle.trie.InsertSerial(serial)

			isle.brood[i][j] = new(probs.ExprReport)
			isle.brood[i][j].SetExpr(e)
			isle.eqnsLog.Println(isle.brood[i][j].Expr())
		}
		isle.eqnsLog.Println()
	}
	isle.eqnsLog.Println()
	isle.eqnsLog.Println()
}

func (isle *EqnIsland) initEval() {
	fmt.Printf("Initial Eqns\n")

	// evaluate new Exprs in brood
	for i := 0; i < isle.numEqns; i++ {
		calcEqnTrainErr(isle.brood[i], isle.prob)
		for j, e := range isle.brood[i] {
			isle.brood[i][j].SetPredError(isle.brood[i][j].TrainError())
			if badEqnFilterTrain(e) {
				isle.brood[i][j] = nil
			}
		}
	}
}
