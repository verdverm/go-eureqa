package gpsr

import (
	"log"
	"math"
	// "fmt"

	// expr "damd/go-symexpr"
	probs "damd/problems"
)

func calcEqnPredErr(eqns probs.ExprReportArray, ssets []*probs.PntSubset, EP *probs.ExprProblem) {
	XN := EP.SearchVar
	for e, E := range eqns {
		TNP := 0
		errSum := 0.0
		hitSum := 0
		for _, S := range ssets {
			DNP := S.NumPoints()
			TNP += DNP
			for p := 0; p < DNP; p++ {
				in := S.Input(p)

				var ret float64
				switch EP.SearchType {
				case probs.ExprBenchmark:
					ret = E.Expr().Eval(0, in.Indeps(), E.Coeff(), S.SysVals())
				case probs.ExprDiffeq:
					ret = E.Expr().Eval(0, in.Indeps()[1:], E.Coeff(), S.SysVals())
				// case probs.ExprDiffeq:
				// 	ret = expr.PRK4(XN, E.Expr(), in.Indep(0), out.Indep(0), in.Indeps()[1:], out.Indeps()[1:], x_tmp, E.Coeff(), S.SysVals())
				// 	dif = (out.Depnd(XN) - in.Depnd(XN))

				default:
					log.Fatalln("Unknown ExprProbleType in CalcEqnPredErr: ", EP.SearchType)
				}

				// TODO parameterize error function
				err := (in.Depnd(XN) - ret)
				if math.IsNaN(err) {
					TNP--
					continue
				}
				aerr := math.Abs(err)

				// TODO parameterize HitRatio to be absolute or percentage
				if aerr < EP.HitRatio {
					hitSum++
				}

				errSum += aerr
			}
		}
		eqns[e].SetPredError(errSum / float64(TNP))
		eqns[e].SetPredScore(hitSum)
	}
	return
}

func calcEqnTrainErr(eqns probs.ExprReportArray, EP *probs.ExprProblem) {
	XN := EP.SearchVar
	for e, E := range eqns {
		TNP := 0
		errSum := 0.0
		hitSum := 0
		perrSum := make([]float64, len(EP.Train))
		phitSum := make([]int, len(EP.Train))
		for d, D := range EP.Train {
			DNP := D.NumPoints()
			TNP += DNP
			for p := 0; p < DNP; p++ {
				in := D.Point(p)
				var ret float64
				switch EP.SearchType {
				case probs.ExprBenchmark:
					ret = E.Expr().Eval(0, in.Indeps(), E.Coeff(), D.SysVals())
				case probs.ExprDiffeq:
					ret = E.Expr().Eval(0, in.Indeps()[1:], E.Coeff(), D.SysVals())
				// ret = expr.PRK4(XN, E.Expr(), in.Indep(0), out.Indep(0), in.Indeps()[1:], out.Indeps()[1:], x_tmp, E.Coeff(), D.SysVals())
				// dif = (out.Depnd(XN) - in.Depnd(XN))
				default:
					log.Fatalln("Unknown ExprProbleType in CalcEqnTrainErr: ", EP.SearchType)
				}

				if math.IsNaN(ret) {
					TNP--
					continue
				}

				// TODO parameterize error function
				err := (in.Depnd(XN) - ret)

				if math.IsNaN(err) {
					TNP--
					continue
				}

				aerr := math.Abs(err)

				// TODO parameterize HitRatio to be absolute or percentage
				if aerr < EP.HitRatio {
					hitSum++
					phitSum[d]++
				}

				errSum += aerr
				perrSum[d] += aerr
			}
			perrSum[d] /= float64(DNP)
		}
		eqns[e].SetTrainError(errSum / float64(TNP))
		eqns[e].SetTrainScore(hitSum)
		eqns[e].SetTrainErrorZ(perrSum)
		eqns[e].SetTrainScoreZ(phitSum)
	}
	return
}

func calcEqnTestErr(eqns probs.ExprReportArray, EP *probs.ExprProblem) {
	XN := EP.SearchVar
	for e, E := range eqns {
		if E == nil {
			continue
		}
		TNP := 0
		errSum := 0.0
		hitSum := 0
		perrSum := make([]float64, len(EP.Test))
		phitSum := make([]int, len(EP.Test))
		for d, D := range EP.Test {
			DNP := D.NumPoints()
			TNP += DNP
			for p := 0; p < DNP; p++ {
				in := D.Point(p)

				var ret float64
				switch EP.SearchType {
				case probs.ExprBenchmark:
					ret = E.Expr().Eval(0, in.Indeps(), E.Coeff(), D.SysVals())
				case probs.ExprDiffeq:
					ret = E.Expr().Eval(0, in.Indeps()[1:], E.Coeff(), D.SysVals())
				default:
					log.Fatalln("Unknown ExprProbleType in CalcEqnTestErr: ", EP.SearchType)
				}

				// TODO parameterize error function
				err := (in.Depnd(XN) - ret)

				if math.IsNaN(err) {
					TNP--
					continue
				}
				aerr := math.Abs(err)

				// TODO parameterize HitRatio to be absolute or percentage
				if aerr < EP.HitRatio {
					hitSum++
					phitSum[d]++
				}

				errSum += aerr
				perrSum[d] += aerr
			}
			perrSum[d] /= float64(DNP)
		}
		eqns[e].SetTestError(errSum / float64(TNP))
		eqns[e].SetTestScore(hitSum)
		eqns[e].SetTestErrorZ(perrSum)
		eqns[e].SetTestScoreZ(phitSum)
	}
	return
}
