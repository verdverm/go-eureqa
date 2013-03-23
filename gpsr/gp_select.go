package gpsr

import (
	"math"
	"sort"

	probs "damd/problems"
)

// returns true when equation
// has Inf or NaN  PredErr
func badEqnFilterPred(eqn *probs.ExprReport) bool {
	if eqn.PredError() > 1e9 ||
		math.IsInf(eqn.PredError(), 0) ||
		math.IsNaN(eqn.PredError()) {
		return true
	}
	return false
}

func badEqnFilterTrain(eqn *probs.ExprReport) bool {
	if eqn.TrainError() > 1e9 ||
		math.IsInf(eqn.TrainError(), 0) ||
		math.IsNaN(eqn.TrainError()) {
		return true
	}
	return false
}

func badEqnFilterTest(eqn *probs.ExprReport) bool {
	if eqn.TestError() > 1e9 ||
		math.IsInf(eqn.TestError(), 0) ||
		math.IsNaN(eqn.TestError()) {
		return true
	}
	return false
}

func (isle *EqnIsland) selectBroodToPareto(st probs.SortType) {
	for i, EA := range isle.brood {
		// Q := probs.NewQueueFromArray(EA)
		// Q.SetSort(st)
		// Q.Sort()
		// isle.pareto[i] = Q.GetReport(0)
		sort.Sort(probs.ExprReportArrayPredError{EA})
		isle.pareto[i] = EA[0]
	}
}
