package gpsr

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	rand "math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	atomic "sync/atomic"
	"time"

	config "damd/config"
	expr "damd/go-symexpr"
	probs "damd/problems"
)

// parameters to a damd search, which sets up the global system 
// and instructs in where to find the sub-searches
type gpsrConfig struct {
	// search params
	maxGen       int
	gpsrRptEpoch int
	gpsrRptCount int

	simprules expr.SimpRules
	treecfg   *probs.TreeParams

	// eqnIsland params
	numEqnIsles   int
	eqnMigEpoch   int
	eqnMigCount   int
	eqnRptEpoch   int
	eqnRptCount   int
	numEqns       int
	eqnBroodSz    int
	eqnCrossRate  float64
	eqnMutateRate float64

	// ssetIsland params
	numSSetIsles   int
	ssetMigEpoch   int
	ssetMigCount   int
	ssetRptEpoch   int
	ssetRptCount   int
	numSSets       int
	ssetSize       int
	ssetBroodSz    int
	ssetCrossRate  float64
	ssetMutateRate float64
}

func gpsrConfigParser(field, value string, config interface{}) (err error) {

	GC := config.(*gpsrConfig)

	switch strings.ToUpper(field) {
	case "MAXGEN":
		GC.maxGen, err = strconv.Atoi(value)
	case "GPSRRPTEPOCH":
		GC.gpsrRptEpoch, err = strconv.Atoi(value)
	case "GPSRRPTCOUNT":
		GC.gpsrRptCount, err = strconv.Atoi(value)

	case "NUMEQNISLES":
		GC.numEqnIsles, err = strconv.Atoi(value)
	case "EQNRPTEPOCH":
		GC.eqnRptEpoch, err = strconv.Atoi(value)
	case "EQNRPTCOUNT":
		GC.eqnRptCount, err = strconv.Atoi(value)
	case "EQNMIGEPOCH":
		GC.eqnMigEpoch, err = strconv.Atoi(value)
	case "EQNMIGCOUNT":
		GC.eqnMigCount, err = strconv.Atoi(value)

	case "NUMEQNS":
		GC.numEqns, err = strconv.Atoi(value)
	case "EQNBROODSZ":
		GC.eqnBroodSz, err = strconv.Atoi(value)
	case "EQNCROSSRATE":
		GC.eqnCrossRate, err = strconv.ParseFloat(value, 64)
	case "EQNMUTATERATE":
		GC.eqnMutateRate, err = strconv.ParseFloat(value, 64)

	case "NUMSSETISLES":
		GC.numSSetIsles, err = strconv.Atoi(value)
	case "SSETRPTEPOCH":
		GC.ssetRptEpoch, err = strconv.Atoi(value)
	case "SSETRPTCOUNT":
		GC.ssetRptCount, err = strconv.Atoi(value)
	case "SSETMIGEPOCH":
		GC.ssetMigEpoch, err = strconv.Atoi(value)
	case "SSETMIGCOUNT":
		GC.ssetMigCount, err = strconv.Atoi(value)

	case "NUMSSETS":
		GC.numSSets, err = strconv.Atoi(value)
	case "SSETSIZE":
		GC.ssetSize, err = strconv.Atoi(value)
	case "SSETBROODSZ":
		GC.ssetBroodSz, err = strconv.Atoi(value)
	case "SSETCROSSRATE":
		GC.ssetCrossRate, err = strconv.ParseFloat(value, 64)
	case "SSETMUTATERATE":
		GC.ssetMutateRate, err = strconv.ParseFloat(value, 64)

	default:
		// check augillary parsable structures [only TreeParams for now]
		if GC.treecfg == nil {
			GC.treecfg = new(probs.TreeParams)
		}
		found, ferr := probs.ParseTreeParams(field, value, GC.treecfg)
		if ferr != nil {
			log.Fatalf("error parsing Problem Config\n")
			return ferr
		}
		if !found {
			log.Printf("GPSR Config Not Implemented: %s, %s\n\n", field, value)
		}

	}
	return
}

type GpsrSearch struct {
	id   int
	gen  int
	cnfg gpsrConfig
	prob *probs.ExprProblem
	pnts *PntStatsArray2d
	rng  *rand.Rand
	stop bool

	// comm upside
	commup *probs.ExprProblemComm

	// comm downside
	// report channels
	eqnCmd    []chan int
	eqnRpt    []chan *probs.ExprReportArray
	eqnGen    chan [2]int // id && gen
	eqnGenCtr []int

	ssetCmd    []chan int
	ssetRpt    []chan []ssetIsleMem
	ssetGen    chan [2]int // id && gen
	ssetGenCtr []int

	// common channels for islands
	eqnMig  []chan *probs.ExprReportArray // each island has its own input, others have copy to send to
	ssetMig []chan [][]ssetIsleMem

	// publishing arrays of channels
	errPub  [](chan *PntStatsArray2d)   // size is [sset_isles] (they are the subscribuers)
	ssetPub [](chan []*probs.PntSubset) // size is [eqn_isles] (they are the subscribuers)

	// best exprs & ssets
	trie      *IpreNode
	eqns      *probs.ReportQueue
	eqnsUnion *probs.ReportQueue
	sset      []ssetIsleMem
	ssetUnion []ssetIsleMem

	per_eqlock int32
	gof_eqlock int32
	per_eqns   []*probs.ExprReportArray
	per_equpd  []int // tracks number of updates since last calc
	per_sslock int32
	per_sset   [][]ssetIsleMem
	per_ssupd  []int

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

	// sub-components
	eqnIsles  []EqnIsland
	ssetIsles []SSetIsland

	// statistics
	neqns    int
	minError float64
}

type GpsrSearchComm struct {
}

func (GS *GpsrSearch) ParseConfig(filename string) {
	fmt.Printf("Parsing GPSR Config: %s\n", filename)
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatal(err)
	}
	err = config.ParseConfig(data, gpsrConfigParser, &GS.cnfg)
	// err = GS.cnfg.parseGpsrConfig(data)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%v\n", GS.cnfg)
	fmt.Printf("%v\n\n", GS.cnfg.treecfg)

}

func (GS *GpsrSearch) Init(done chan int, prob *probs.ExprProblem, logdir string, input interface{}) {
	fmt.Printf("Init'n GPSR\n--------------\n")
	GS.rng = rand.New(rand.NewSource(rand.Int63()))

	GS.minError = 10000000.0

	GS.trie = new(IpreNode)
	GS.trie.val = -1
	GS.trie.next = make(map[int]*IpreNode)

	GS.initLogs(logdir)

	// copy in data and common config options
	GS.prob = prob
	if GS.cnfg.treecfg == nil {
		GS.cnfg.treecfg = GS.prob.TreeCfg.Clone()
	}
	srules := expr.DefaultRules()
	srules.ConvertConsts = false
	GS.cnfg.treecfg.SRules = srules

	fmt.Println("GS.cnfg:", GS.cnfg)
	fmt.Println("GS.treecfg", GS.cnfg.treecfg)
	fmt.Println("GS.prob", GS.prob)
	fmt.Println()

	// copy in upward comm struct
	GS.commup = input.(*probs.ExprProblemComm)

	GS.trie = new(IpreNode)
	GS.trie.val = -1
	GS.trie.next = make(map[int]*IpreNode)

	GS.initEqnIsles()

	// get initial equations
	fmt.Println("Checking Init Eqn Messages")
	GS.checkMessages()

	// merge and sort eqns
	fmt.Println("Accumulating Init'd Eqns")
	GS.accumExprReports()

	// calcPntErrors
	GS.pnts = calcEqnErrs(GS.eqns.GetQueue(), GS.prob)

	// initialize errors (sset init grabs the GS.pnts)
	GS.initSSetIsles()

	GS.checkMessages()

	// GS.ssetIsles[0].step()

	// merge and sort ssets
	GS.accumSSets()

	// report SSets to EqnIsles
	GS.publishSSets()

	// GS.eqnIsles[0].step()

	// done <- 0
}

func (GS *GpsrSearch) Run() {
	fmt.Printf("Running GPSR\n")

	for i := 0; i < len(GS.eqnIsles); i++ {
		go GS.eqnIsles[i].run()
	}
	for i := 0; i < len(GS.ssetIsles); i++ {
		go GS.ssetIsles[i].run()
	}
	GS.loop()

	GS.commup.Cmds <- -1
}

func (GS *GpsrSearch) loop() {
	counter := 0
	doRpt := false
	for !GS.stop {
		// fmt.Println("GS: ", counter)

		eSum := 0
		for _, g := range GS.eqnGenCtr {
			eSum += g
		}
		eSum /= len(GS.eqnGenCtr)
		if eSum > GS.gen {
			GS.gen = eSum
			// fmt.Println("gen: ", GS.gen)
			GS.commup.Gen <- [2]int{GS.id, GS.gen}
			doRpt = true
		}

		if doRpt && GS.gen%GS.cnfg.gpsrRptEpoch == 0 {
			doRpt = false
			GS.mainLog.Printf("GpsrSearch:%d reporting eqns at gen %d\n", GS.id, GS.gen)
			GS.reportExprs()
		}

		counter++
		// time.Sleep(time.Second / 20)

		GS.checkMessages()

	}
	GS.Clean()
	fmt.Println("GS Exiting ", GS.id)
}

func (GS *GpsrSearch) Clean() {
	GS.mainLog.Printf("Cleaning GPSR\n")

	GS.errLogBuf.Flush()
	GS.mainLogBuf.Flush()
	GS.eqnsLogBuf.Flush()
	GS.ssetLogBuf.Flush()
	GS.fitnessLogBuf.Flush()
}

func (GS *GpsrSearch) initLogs(logdir string) {
	// open logs
	GS.logDir = logdir + "gpsr/"
	os.Mkdir(GS.logDir, os.ModePerm)
	tmpF0, err5 := os.Create(GS.logDir + "gpsr:err.log")
	if err5 != nil {
		log.Fatal("couldn't create errs log")
	}
	GS.errLogBuf = bufio.NewWriter(tmpF0)
	GS.errLogBuf.Flush()
	GS.errLog = log.New(GS.errLogBuf, "", log.LstdFlags)

	tmpF1, err1 := os.Create(GS.logDir + "gpsr:main.log")
	if err1 != nil {
		log.Fatal("couldn't create main log")
	}
	GS.mainLogBuf = bufio.NewWriter(tmpF1)
	GS.mainLogBuf.Flush()
	GS.mainLog = log.New(GS.mainLogBuf, "", log.LstdFlags)

	tmpF2, err2 := os.Create(GS.logDir + "gpsr:eqns.log")
	if err2 != nil {
		log.Fatal("couldn't create eqns log")
	}
	GS.eqnsLogBuf = bufio.NewWriter(tmpF2)
	GS.eqnsLogBuf.Flush()
	GS.eqnsLog = log.New(GS.eqnsLogBuf, "", 0)

	tmpF3, err3 := os.Create(GS.logDir + "gpsr:sset.log")
	if err3 != nil {
		log.Fatal("couldn't create ssets log")
	}
	GS.ssetLogBuf = bufio.NewWriter(tmpF3)
	GS.ssetLogBuf.Flush()
	GS.ssetLog = log.New(GS.ssetLogBuf, "", log.LstdFlags)

	tmpF5, err5 := os.Create(GS.logDir + "gpsr:fitness.log")
	if err5 != nil {
		log.Fatal("couldn't create eqns log")
	}
	GS.fitnessLogBuf = bufio.NewWriter(tmpF5)
	GS.fitnessLog = log.New(GS.fitnessLogBuf, "", log.Ltime|log.Lmicroseconds)
	GS.fitnessLogBuf.Flush()

}

func (GS *GpsrSearch) initEqnIsles() {
	// EqnIslands
	fmt.Println("Initializing EqnIslands\n---------------------")
	// setup downward comm struct
	GS.eqnCmd = make([]chan int, GS.cnfg.numEqnIsles)
	GS.eqnRpt = make([]chan *probs.ExprReportArray, GS.cnfg.numEqnIsles)
	GS.eqnMig = make([]chan *probs.ExprReportArray, GS.cnfg.numEqnIsles)
	GS.ssetPub = make([]chan []*probs.PntSubset, GS.cnfg.numEqnIsles)
	GS.per_eqns = make([]*probs.ExprReportArray, GS.cnfg.numEqnIsles)
	for i, _ := range GS.eqnRpt {
		GS.eqnCmd[i] = make(chan int)
		GS.eqnRpt[i] = make(chan *probs.ExprReportArray, 128)
		GS.eqnMig[i] = make(chan *probs.ExprReportArray, 256)
		GS.ssetPub[i] = make(chan []*probs.PntSubset, 64)
	}
	GS.eqnGen = make(chan [2]int, 256*GS.cnfg.numEqnIsles)
	GS.eqnGenCtr = make([]int, GS.cnfg.numEqnIsles)
	GS.per_equpd = make([]int, GS.cnfg.numEqnIsles)

	// setup eqnislands
	GS.eqnIsles = make([]EqnIsland, GS.cnfg.numEqnIsles)
	for i, _ := range GS.eqnIsles {
		GS.eqnIsles[i].id = i
		GS.eqnIsles[i].copyInConfig(GS)
		GS.eqnIsles[i].init()
	}
	fmt.Println()

}

func (GS *GpsrSearch) initSSetIsles() {
	// SSetIslands
	if GS.cnfg.numSSetIsles > 0 {
		fmt.Println("Initializing SSetIslands\n---------------------")
		// setup downward comm struct
		GS.ssetCmd = make([]chan int, GS.cnfg.numSSetIsles)
		GS.errPub = make([]chan *PntStatsArray2d, GS.cnfg.numSSetIsles)
		GS.ssetRpt = make([]chan []ssetIsleMem, GS.cnfg.numSSetIsles)
		GS.ssetMig = make([]chan [][]ssetIsleMem, GS.cnfg.numSSetIsles)
		GS.per_sset = make([][]ssetIsleMem, GS.cnfg.numSSetIsles)
		for i, _ := range GS.ssetRpt {
			fmt.Printf("SSetIsland %d\n", i)
			GS.ssetCmd[i] = make(chan int)
			GS.errPub[i] = make(chan *PntStatsArray2d, 16)
			GS.ssetRpt[i] = make(chan []ssetIsleMem, 128)
			GS.ssetMig[i] = make(chan [][]ssetIsleMem, 128)
			GS.per_sset[i] = make([]ssetIsleMem, 0)
		}
		GS.ssetGen = make(chan [2]int, 1024*GS.cnfg.numSSetIsles)
		GS.ssetGenCtr = make([]int, GS.cnfg.numSSetIsles)
		GS.per_ssupd = make([]int, GS.cnfg.numSSetIsles)

		// setup ssetislands
		GS.ssetIsles = make([]SSetIsland, GS.cnfg.numSSetIsles)
		for i, _ := range GS.ssetIsles {
			GS.ssetIsles[i].id = i
			GS.ssetIsles[i].copyInConfig(GS)
			GS.ssetIsles[i].init()
		}
	}
	fmt.Println()

}

var firstMsgCheck = true

func (GS *GpsrSearch) checkMessages() {

	// check messages from superior
	if !GS.stop {
		select {
		case cmd, ok := <-GS.commup.Cmds:
			if ok {
				if cmd == -1 {
					GS.stop = true
					GS.doStop()
				}
			}
		default:
			goto nocmd
		}
	nocmd:
	}

	// check generations 
	for {
		select {
		case egen, ok := <-GS.eqnGen:
			if ok {
				GS.eqnGenCtr[egen[0]] = egen[1]
			}
		case sgen, ok := <-GS.ssetGen:
			if ok {
				GS.ssetGenCtr[sgen[0]] = sgen[1]
			}
		default:
			goto nogen
		}
	}
nogen:

	// check for equations

	for i := 0; i < len(GS.per_eqns); i++ {
		select {
		case eqns, ok := <-GS.eqnRpt[i]:
			if ok {
				for !atomic.CompareAndSwapInt32(&GS.per_eqlock, 0, 1) {
					time.Sleep(time.Microsecond)
				}
				GS.per_eqns[i] = eqns
				atomic.StoreInt32(&GS.per_eqlock, 0)

				GS.per_equpd[i]++

				i-- // so we get the latest report
			}
		// so we don't wait indefinitely for msg in select
		default:
			continue
		}
	}

	peSum := 0
	for _, s := range GS.per_equpd {
		peSum += s
	}

	if peSum >= len(GS.per_equpd) && peSum > 0 && !firstMsgCheck {
		for i, _ := range GS.per_equpd {
			GS.per_equpd[i] = 0
		}
		go func() {
			for !atomic.CompareAndSwapInt32(&GS.gof_eqlock, 0, 1) {
				time.Sleep(time.Microsecond)
			}
			GS.accumExprReports()
			GS.pnts = calcEqnErrs(GS.eqns.GetQueue(), GS.prob)
			GS.publishErrors()
			// GS.reportExprs()
			atomic.StoreInt32(&GS.gof_eqlock, 0)

		}()

	}

	// check for subsets
	for i := 0; i < len(GS.per_sset); i++ {
		select {
		case sset, ok := <-GS.ssetRpt[i]:
			if ok {
				// fmt.Println("sset comm ", len(sset))
				GS.per_sset[i] = sset
				GS.per_ssupd[i]++
				i-- // to get the latest
			}
		// so we don't wait indefinitely for msg in select
		default:
			continue
		}
	}

	psSum := 0
	for _, s := range GS.per_ssupd {
		psSum += s
	}
	if psSum >= len(GS.per_ssupd) && psSum > 0 && !firstMsgCheck {
		for i, _ := range GS.per_ssupd {
			GS.per_ssupd[i] = 0
		}
		GS.accumSSets()
		GS.publishSSets()
	}

	firstMsgCheck = false
}

func (GS *GpsrSearch) accumExprReports() {
	if GS.eqnsUnion == nil {
		// the plus 1 is for the already accumulated eqns in GS.eqns
		tmp := make(probs.ExprReportArray, (GS.cnfg.numEqnIsles+1)*GS.cnfg.numEqns)
		GS.eqnsUnion = probs.NewQueueFromArray(tmp)
		GS.eqnsUnion.SetSort(probs.GPSORT_PARETO_TST_ERR)
	}
	if GS.eqns == nil {
		tmp := make(probs.ExprReportArray, GS.cnfg.numEqns)
		GS.eqns = probs.NewQueueFromArray(tmp)
		GS.eqns.SetSort(probs.GPSORT_PARETO_TST_ERR)
	}

	// fill union
	for !atomic.CompareAndSwapInt32(&GS.per_eqlock, 0, 1) {
		time.Sleep(time.Microsecond)
	}
	union := GS.eqnsUnion.GetQueue()
	p1, p2 := 0, 0
	for i := 0; i < GS.cnfg.numEqnIsles; i++ {
		p1 = i * GS.cnfg.numEqns
		p2 = p1 + GS.cnfg.numEqns
		copy(union[p1:p2], *(GS.per_eqns[i]))
	}

	p1 = p2
	p2 += GS.cnfg.numEqns
	copy(union[p1:p2], GS.eqns.GetQueue())
	atomic.StoreInt32(&GS.per_eqlock, 0)

	// remove duplicates
	sort.Sort(union)

	// ipretree accounting
	for _, r := range union {
		if r == nil || r.Expr() == nil {
			continue
		}
		GS.neqns++
		e := r.Expr()
		serial := make([]int, 0, 64)
		serial = e.Serial(serial)
		GS.trie.InsertSerial(serial)

	}

	last := 0
	for union[last] == nil {
		last++
	}
	for i := last + 1; i < len(union); i++ {
		if union[i] == nil {
			continue
		}
		if union[i].Expr().AmIAlmostSame(union[last].Expr()) {
			union[i] = nil
		} else {
			last = i
		}
	}

	// evaluate union members on test data
	calcEqnTestErr(union, GS.prob)

	errSum, errCnt := 0.0, 0
	for _, r := range union {
		if r == nil {
			continue
		}
		if r.TestError() < GS.minError {
			GS.minError = r.TestError()
		}
		errSum += r.TestError()
		errCnt++
	}

	GS.fitnessLog.Println(GS.gen, GS.neqns, GS.trie.cnt, GS.trie.vst, errSum/float64(errCnt), GS.minError)

	// pareto sort union by test error
	GS.eqnsUnion.Sort()

	// copy |GS.cnfg.numEqns| from union to GS.eqns
	copy(GS.eqns.GetQueue(), GS.eqnsUnion.GetQueue()[:GS.cnfg.numEqns])

	GS.eqns.Sort()
}

func (GS *GpsrSearch) publishErrors() {
	GS.mainLog.Println("publishing Errors")
	for _, ep := range GS.errPub {
		// fmt.Println(i)
		ep <- GS.pnts
	}
}

func (GS *GpsrSearch) accumSSets() {
	if GS.ssetUnion == nil {
		GS.ssetUnion = make([]ssetIsleMem, (GS.cnfg.numSSetIsles+1)*GS.cnfg.ssetRptCount)
	}
	if GS.sset == nil {
		GS.sset = make([]ssetIsleMem, GS.cnfg.ssetRptCount)
	}

	union := GS.ssetUnion
	p1, p2 := 0, 0
	for i := 0; i < GS.cnfg.numSSetIsles; i++ {
		p1 = i * GS.cnfg.ssetRptCount
		p2 = p1 + GS.cnfg.ssetRptCount
		// fmt.Println("SSet Accum: ", i, p1, p2)
		// fmt.Println(len(GS.per_sset), len(union))
		copy(union[p1:p2], GS.per_sset[i])
	}
	p1 = p2
	p2 += GS.cnfg.ssetRptCount
	copy(union[p1:p2], GS.sset)

	// for i, u := range union {
	// 	fmt.Println("union ", i, u)
	// }

	if GS.rng.Intn(2) == 0 {
		sort.Sort(ssetVarArray{union})
		copy(GS.sset, union[:GS.cnfg.ssetRptCount])
		sort.Sort(ssetErrArray(GS.sset))
	} else {
		sort.Sort(ssetErrArray(union))
		copy(GS.sset, union[:GS.cnfg.ssetRptCount])
		sort.Sort(ssetVarArray{GS.sset})
	}

}

func (GS *GpsrSearch) publishSSets() {
	pub := make([]*probs.PntSubset, GS.cnfg.ssetRptCount)
	for i := 0; i < GS.cnfg.ssetRptCount; i++ {
		p := GS.rng.Intn(len(GS.sset))
		pub[i] = makeSubsetFromMem(GS.prob, GS.sset[p])
	}
	for _, sp := range GS.ssetPub {
		sp <- pub
	}
}

func (GS *GpsrSearch) reportExprs() {
	cnt := GS.cnfg.gpsrRptCount
	rpt := make(probs.ExprReportArray, cnt)
	copy(rpt, GS.eqns.GetQueue()[:cnt])

	GS.eqnsLog.Println("GEN: ", GS.gen, len(rpt))

	for i, r := range rpt {
		GS.eqnsLog.Println(i, r.Expr().Latex(GS.prob.Train[0].GetIndepNames(), GS.prob.Train[0].GetSysNames(), r.Coeff()))
		GS.eqnsLog.Println(r, "\n")
	}

	GS.commup.Rpts <- &rpt
}

func (GS *GpsrSearch) doStop() {
	num_subs := len(GS.eqnIsles) + len(GS.ssetIsles)
	done := make(chan int, num_subs)

	for i, _ := range GS.eqnCmd {
		func() {
			C := GS.eqnCmd[i]
			c := i
			go func() {
				C <- -1
				fmt.Printf("sent -1 to eqn %d\n", c)
				<-C
				done <- 1
			}()
		}()
	}
	for i, _ := range GS.ssetCmd {
		func() {
			C := GS.ssetCmd[i]
			c := i
			go func() {
				C <- -1
				fmt.Printf("sent -1 to sset %d\n", c)
				<-C
				done <- 1
			}()
		}()
	}

	cnt := 0
	for cnt < num_subs {
		GS.checkMessages()
		// select {
		// case _, ok := <-done:
		// 	if ok {
		// 		cnt++
		// 		fmt.Println("GS.done.cnt = ", cnt, num_subs)
		// 	}
		// default:
		// 	continue
		// }
		_, ok := <-done
		if ok {
			cnt++
			fmt.Println("GS.done.cnt = ", cnt, num_subs)
		}

	}
	fmt.Println("GS: Done Killing")
}
