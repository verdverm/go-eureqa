package gpsr

import (
	"bufio"
	"fmt"
	"log"
	"math"
	rand "math/rand"
	"os"
	"sort"

	probs "damd/problems"
)

// struct to hold the calculated values of a single point
// across a set of equations
type PntStats struct {
	AveErr, Variance, MinErr, MaxErr float64
}
type PntStatsArray2d [][]PntStats // |dataSets|*|numPts| over |eqns|

type ssetIsleMem struct {
	dataset   int
	indices   []int
	err, vari float64
}

func makeSubsetFromMem(prob *probs.ExprProblem, mem ssetIsleMem) *probs.PntSubset {
	ps := new(probs.PntSubset)
	ps.SetDS(prob.Train[mem.dataset])
	ps.SetIndexes(mem.indices)
	ps.Refresh()
	return ps
}

type ssetErrArray []ssetIsleMem

func (s ssetErrArray) Len() int { return len(s) }
func (s ssetErrArray) Less(i, j int) bool {
	if len(s[i].indices) == 0 {
		return false
	}
	if len(s[j].indices) == 0 {
		return true
	}
	return s[i].err < s[j].err
}
func (s ssetErrArray) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

type ssetVarArray struct {
	ssetErrArray
}

func (s ssetVarArray) Less(i, j int) bool {
	if len(s.ssetErrArray[i].indices) == 0 {
		return false
	}
	if len(s.ssetErrArray[j].indices) == 0 {
		return true
	}
	return s.ssetErrArray[i].vari > s.ssetErrArray[j].vari
}

type SSetIsland struct {
	id   int
	gen  int
	rng  *rand.Rand
	stop bool

	// data & evaluation
	prob *probs.ExprProblem
	pnts *PntStatsArray2d

	// communications stuff
	ssetCmd chan int
	pntErrs chan *PntStatsArray2d //, eqns we are optimizing subsets for ???

	ssetInMig  chan [][]ssetIsleMem
	ssetOutMig []chan [][]ssetIsleMem

	ssetRpt chan []ssetIsleMem
	ssetGen chan [2]int // id && gen

	// common config options
	ssetMigEpoch int
	ssetMigCount int
	ssetRptEpoch int
	ssetRptCount int

	numSSets    int
	ssetSize    int
	ssetBroodSz int
	crossRate   float64
	mutateRate  float64

	// extra config options

	// internal data [extra dimension is for data set]
	// we want subsets to be specific to data sets, and able to vary indices
	parents  [][]ssetIsleMem   // |datasets|*|numSSets|
	brood    [][][]ssetIsleMem // |datasets|*|ssetBroodSz|*|numSSet|
	pareto   [][]ssetIsleMem   // |datasets|*|numSSets|
	migrants [][]ssetIsleMem

	// logs
	logDir     string
	mainLog    *log.Logger
	mainLogBuf *bufio.Writer
	ssetLog    *log.Logger
	ssetLogBuf *bufio.Writer
	eqnsLog    *log.Logger
	eqnsLogBuf *bufio.Writer // eqns we are optimizing subsets for ???
	errLog     *log.Logger
	errLogBuf  *bufio.Writer
}

func (isle *SSetIsland) copyInConfig(gs *GpsrSearch) {
	gp := gs.cnfg
	isle.pnts = gs.pnts
	isle.logDir = gs.logDir + fmt.Sprintf("sisle%d/", isle.id)
	isle.prob = gs.prob

	isle.numSSets = gp.numSSets
	isle.ssetBroodSz = gp.ssetBroodSz
	isle.ssetSize = gp.ssetSize
	isle.ssetMigEpoch = gp.ssetMigEpoch
	isle.ssetMigCount = gp.ssetMigCount
	isle.ssetRptEpoch = gp.ssetRptEpoch
	isle.ssetRptCount = gp.ssetRptCount
	isle.crossRate = gp.ssetCrossRate
	isle.mutateRate = gp.ssetMutateRate

	isle.ssetCmd = gs.ssetCmd[isle.id]
	isle.pntErrs = gs.errPub[isle.id]
	isle.ssetInMig = gs.ssetMig[isle.id]
	if gp.numSSetIsles > 1 {
		isle.migrants = make([][]ssetIsleMem, len(isle.prob.Train))
		isle.ssetOutMig = make([]chan [][]ssetIsleMem, 2)
		m1 := ((isle.id + gp.numSSetIsles - 1) % gp.numSSetIsles)
		p1 := ((isle.id + 1) % gp.numSSetIsles)
		isle.ssetOutMig[0] = gs.ssetMig[m1]
		isle.ssetOutMig[1] = gs.ssetMig[p1]
	}

	isle.ssetRpt = gs.ssetRpt[isle.id]
	isle.ssetGen = gs.ssetGen

}

func (isle *SSetIsland) init() {
	fmt.Println("Initializing SSetIsland ", isle.id)

	// create random number generator
	isle.rng = rand.New(rand.NewSource(rand.Int63()))

	// open logs
	isle.initLogs()
	isle.mainLog.Println("Initializing SSetIsland ", isle.id)

	// create random subsets
	isle.initSubsets()

	// eval initial subsets
	isle.eval()

	// select initial subsets
	isle.selecting()

	// report best subsets
	rpt := make([]ssetIsleMem, isle.ssetMigCount)
	for i := 0; i < isle.ssetMigCount; i++ {
		// randomly select dataset
		drng := isle.rng.Intn(len(isle.pareto))
		// randomly select an early subset
		prng := isle.rng.Intn(isle.numSSets)
		rpt[i] = isle.pareto[drng][prng]
	}
	isle.ssetRpt <- rpt

	// flush logs at end of init
	isle.flushLogs()
}

func (isle *SSetIsland) run() {
	fmt.Println("Running SSetIsland ", isle.id)

	isle.messages()
	for !isle.stop {
		isle.step()
		isle.messages()
	}
	fmt.Println("SSetIsle", isle.id, " exiting")
	isle.ssetCmd <- -1
}

func (isle *SSetIsland) clean() {
	isle.mainLog.Println("Cleaning SSetIsland ", isle.id)

	isle.flushLogs()
}

func (isle *SSetIsland) step() {

	isle.eval()
	isle.selecting()

	isle.gen++

	isle.report()
	isle.migrate()
	isle.breed()

	isle.messages()
	isle.ssetGen <- [2]int{isle.id, isle.gen}
	isle.flushLogs()

}

func (isle *SSetIsland) messages() {
	// isle.mainLog.Println("Messages SSetIsland ", isle.id, isle.gen)
	for {
		select {
		// check upstream messages 
		case cmd, ok := <-isle.ssetCmd:
			if ok {
				switch cmd {
				case -1:
					// end processing
					fmt.Println("SSetIsle", isle.id, " Stopping  !!!!!!!!!!")
					isle.stop = true
					return
				}
			}

		// check for immigrants
		case migs, ok := <-isle.ssetInMig:
			if ok {
				for d, _ := range migs {
					isle.migrants[d] = append(isle.migrants[d], migs[d][:]...)
				}
			}

		case pnts, ok := <-isle.pntErrs:
			if ok && pnts != nil {
				isle.mainLog.Println("New isle.pnts ", isle.id, isle.gen)
				isle.pnts = pnts
			}
		default:
			return // so we don't wait indefinitely for msg in select
		}
	}
}
func (isle *SSetIsland) eval() {
	// isle.mainLog.Println("Evaluating SSetIsland ", isle.id, isle.gen)

	div := float64(isle.ssetSize)

	for d, D := range *(isle.pnts) {
		for b, B := range isle.brood[d] { // brood index
			for m, M := range B { // brood member
				eSum, vSum := 0.0, 0.0
				for _, I := range M.indices { // member pnt index
					eSum += D[I].AveErr
					vSum += D[I].Variance
				}
				isle.brood[d][b][m].err = eSum / div
				isle.brood[d][b][m].vari = vSum / div
			}
		}
	}
}
func (isle *SSetIsland) selecting() {
	// isle.mainLog.Println("Selecting SSetIsland ", isle.id, isle.gen)

	// per data set 
	for d, D := range isle.brood {
		for b, B := range D {
			// select within brood to pareto-ish
			min := 0
			for m, M := range B {
				if isle.gen%2 == 0 {
					if M.err < B[min].err {
						min = m
					}
				} else {
					if M.vari > B[min].vari {
						min = m
					}
				}
			}
			isle.pareto[d][b] = B[min]

		}

		pLen := len(isle.pareto[d])

		// add migrants to pareto-ish
		if len(isle.migrants) > 0 {
			isle.pareto[d] = append(isle.pareto[d], isle.migrants[d]...)
		}

		// sort pareto-ish (opposite of brood best)
		if isle.gen%2 == 0 {
			sort.Sort(ssetVarArray{isle.pareto[d]})
		} else {
			sort.Sort(ssetErrArray(isle.pareto[d]))
		}

		// select for parents
		copy(isle.parents[d], isle.pareto[d])

		// reset len of pareto[d]
		isle.pareto[d] = isle.pareto[d][:pLen]
	}
}
func (isle *SSetIsland) report() {

	// send ssets
	if isle.gen%isle.ssetRptEpoch == 0 {
		isle.mainLog.Println("Reporting SSetIsland ", isle.id, isle.gen)
		rpt := make([]ssetIsleMem, isle.ssetRptCount)
		for i := 0; i < isle.ssetRptCount; i++ {
			// randomly select dataset
			drng := isle.rng.Intn(len(isle.pareto))
			// randomly select an early subset
			prng := isle.rng.Intn(isle.ssetRptCount)
			rpt[i] = isle.pareto[drng][prng]
		}
		isle.ssetRpt <- rpt
	}

}

func (isle *SSetIsland) migrate() {
	ND := len(isle.prob.Train)
	// migrate ssets
	if isle.gen%isle.ssetMigEpoch == 0 && len(isle.ssetOutMig) > 0 {
		// isle.mainLog.Println("Migrating SSetIsland ", isle.id, isle.gen)

		rpt := make([][]ssetIsleMem, ND)
		for d := 0; d < ND; d++ {
			rpt[d] = make([]ssetIsleMem, isle.ssetMigCount)
			for i := 0; i < isle.ssetMigCount; i++ {
				// randomly select a subset
				prng := isle.rng.Intn((len(isle.pareto[d]) * 3) / 2)
				rpt[d][i] = isle.pareto[d][prng]
			}
		}
		for _, comm := range isle.ssetOutMig {
			comm <- rpt
		}
	}
}
func (isle *SSetIsland) breed() {
	// isle.mainLog.Println("Breeding SSetIsland ", isle.id, isle.gen)
	SS := isle.ssetSize
	NS := isle.numSSets
	for d := 0; d < len(isle.parents); d++ {
		NP := isle.prob.Train[d].NumPoints()
		for i := 0; i < isle.numSSets; i++ {
			rnum1, rnum2, rnum3, rnum4 := isle.rng.Intn(NS), isle.rng.Intn(NS), isle.rng.Intn(NS), isle.rng.Intn(NS)
			if rnum3 < rnum1 {
				rnum1 = rnum3
			}
			if rnum4 < rnum2 {
				rnum2 = rnum4
			}
			s1 := isle.parents[d][rnum1]
			s2 := isle.parents[d][rnum3]

			for b := 0; b < isle.ssetBroodSz; b++ {
				var sset ssetIsleMem
				sset.dataset = d
				sset.indices = make([]int, isle.ssetSize)
				if isle.rng.Float64() < isle.crossRate {
					// crossSubsets
					P := isle.rng.Intn(SS-2) + 1 // so we don't pick the end points
					// fmt.Println(P, "/", SS)
					copy(sset.indices[:P], s1.indices[:P])
					copy(sset.indices[P:], s2.indices[P:])
				} else {
					// injectSubset  ( 2 point splice )
					// P1, P2 := SSLP.rng.Intn(SS), SSLP.rng.Intn(SS)
					// if P2 < P1 {
					// 	P1, P2 = P2, P1
					// }
					// copy(new_idx[:P1], s1.SS.Indexes()[:P1])
					// copy(new_idx[P2:], s1.SS.Indexes()[P2:])
					// for p := P1; p < P2; p++ {
					// 	new_idx[p] = SSLP.rng.Int() % (SSLP.data[D].NumPoints() - 1)
					// }
				}

				if isle.rng.Float64() < isle.mutateRate {
					pos := isle.rng.Intn(SS)
					newP := isle.rng.Intn(NP)
					sset.indices[pos] = newP
				}
				isle.brood[d][i][b] = sset
			}
		}
	}

}

func (isle *SSetIsland) flushLogs() {
	isle.errLogBuf.Flush()
	isle.mainLogBuf.Flush()
	isle.ssetLogBuf.Flush()
	isle.eqnsLogBuf.Flush()
}

func (isle *SSetIsland) initLogs() {
	os.Mkdir(isle.logDir, os.ModePerm)
	tmpF0, err5 := os.Create(isle.logDir + fmt.Sprintf("sisle%d:err.log", isle.id))
	if err5 != nil {
		log.Fatal("couldn't create errs log")
	}
	isle.errLogBuf = bufio.NewWriter(tmpF0)
	isle.errLog = log.New(isle.errLogBuf, "", log.LstdFlags)

	tmpF1, err1 := os.Create(isle.logDir + fmt.Sprintf("sisle%d:main.log", isle.id))
	if err1 != nil {
		log.Fatal("couldn't create main log")
	}
	isle.mainLogBuf = bufio.NewWriter(tmpF1)
	isle.mainLog = log.New(isle.mainLogBuf, "", log.LstdFlags)

	tmpF3, err3 := os.Create(isle.logDir + fmt.Sprintf("sisle%d:sset.log", isle.id))
	if err3 != nil {
		log.Fatal("couldn't create ssets log")
	}
	isle.ssetLogBuf = bufio.NewWriter(tmpF3)
	isle.ssetLog = log.New(isle.ssetLogBuf, "", log.LstdFlags)

	tmpF4, err4 := os.Create(isle.logDir + fmt.Sprintf("sisle%d:eqns.log", isle.id))
	if err4 != nil {
		log.Fatal("couldn't create eqns log")
	}
	isle.eqnsLogBuf = bufio.NewWriter(tmpF4)
	isle.eqnsLog = log.New(isle.eqnsLogBuf, "", log.LstdFlags)

}

func (isle *SSetIsland) initSubsets() {
	npts := isle.prob.Train[0].NumPoints()

	// initialize empty parents and pareto 
	isle.parents = make([][]ssetIsleMem, len(*(isle.pnts)))
	isle.pareto = make([][]ssetIsleMem, len(*(isle.pnts)))
	for d := 0; d < len(*(isle.pnts)); d++ {
		isle.parents[d] = make([]ssetIsleMem, isle.numSSets)
		isle.pareto[d] = make([]ssetIsleMem, isle.numSSets)
		for i := 0; i < isle.numSSets; i++ {
			isle.parents[d][i].indices = make([]int, isle.ssetSize)
			isle.pareto[d][i].indices = make([]int, isle.ssetSize)
		}
	}

	// initialize new subsets into brood
	fmt.Printf("Brood: %d %d %d %d\n", len(*isle.pnts), isle.numSSets, isle.ssetBroodSz, isle.ssetSize)

	isle.brood = make([][][]ssetIsleMem, len(*(isle.pnts)))
	for d := 0; d < len(*(isle.pnts)); d++ {
		isle.brood[d] = make([][]ssetIsleMem, isle.numSSets)
		for i := 0; i < isle.numSSets; i++ {
			isle.ssetLog.Println("SSetIsle Init Brood ", d, i)
			isle.brood[d][i] = make([]ssetIsleMem, isle.ssetBroodSz)
			for j := 0; j < isle.ssetBroodSz; j++ {
				isle.brood[d][i][j].dataset = d
				isle.brood[d][i][j].indices = make([]int, isle.ssetSize)
				for k := 0; k < isle.ssetSize; k++ {
					isle.brood[d][i][j].indices[k] = rand.Intn(npts - 1)
				}
				isle.ssetLog.Println(isle.brood[d][i][j])
			}
			isle.ssetLog.Println()
		}
	}
}

func newPntStats(EP *probs.ExprProblem) *PntStatsArray2d {
	PS := make(PntStatsArray2d, len(EP.Train))
	for i := 0; i < len(EP.Train); i++ {
		PS[i] = make([]PntStats, EP.Train[i].NumPoints())
	}
	return &PS
}

func calcEqnErrs(eqns probs.ExprReportArray, EP *probs.ExprProblem) (PS *PntStatsArray2d) {

	PS = newPntStats(EP)
	XN := EP.SearchVar

	Err := make([]float64, len(eqns))
	for d, D := range EP.Train {
		DNP := D.NumPoints()
		for p := 0; p < DNP; p++ {
			in := D.Point(p)

			Stat := (*PS)[d][p]
			Stat.MinErr = math.Inf(1)
			for e, E := range eqns {
				var ret float64
				switch EP.SearchType {
				case probs.ExprBenchmark:
					ret = E.Expr().Eval(0, in.Indeps(), E.Coeff(), D.SysVals())
				case probs.ExprDiffeq:
					ret = E.Expr().Eval(0, in.Indeps()[1:], E.Coeff(), D.SysVals())
				default:
					log.Fatalln("Unknown ExprProbleType in CalcEqnErrs: ", EP.SearchType)
				}
				err := (in.Depnd(XN) - ret)
				if math.IsNaN(err) {
					continue
				}

				aerr := math.Abs(err)
				Err[e] = aerr
				Stat.AveErr += aerr
				if aerr < Stat.MinErr {
					Stat.MinErr = aerr
				}
				if aerr > Stat.MaxErr {
					Stat.MaxErr = aerr
				}
			}
			Stat.AveErr /= float64(len(eqns))
			// calc StdDev
			sum := 0.
			for e := 0; e < len(Err); e++ {
				dif := Err[e] - Stat.AveErr
				sum += dif * dif
			}
			Stat.Variance = sum
			(*PS)[d][p] = Stat
		}
	}
	return
}
