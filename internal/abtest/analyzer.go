package abtest

import (
	"fmt"
	"math"
	"sort"
)

// AnalysisResult is the full statistical comparison of an experiment's variants.
type AnalysisResult struct {
	TestID         string                        `json:"test_id"`
	Status         Status                        `json:"status"`
	Winner         string                        `json:"winner,omitempty"`
	Results        map[string]*VariantResult     `json:"results"`
	Significance   map[string]*MetricSignificance `json:"statistical_significance,omitempty"`
	Recommendation string                        `json:"recommendation,omitempty"`
}

// VariantResult holds summarized metrics for display.
type VariantResult struct {
	Model             string  `json:"model"`
	Samples           int     `json:"samples"`
	LatencyP95Ms      float64 `json:"latency_p95_ms"`
	AvgCostPerRequest float64 `json:"avg_cost_per_request"`
	ErrorRate         float64 `json:"error_rate"`
}

// MetricSignificance holds the statistical test outcome for one metric.
type MetricSignificance struct {
	PValue         float64 `json:"p_value"`
	Significant    bool    `json:"significant"`
	ImprovementPct float64 `json:"improvement_pct"`
}

// Analyze compares control (TrafficSplit[0]) vs treatment (TrafficSplit[1]).
// stats maps variant name → VariantStats collected from the DB.
func Analyze(exp *Experiment, stats map[string]*VariantStats) *AnalysisResult {
	res := &AnalysisResult{
		TestID:  exp.ID,
		Status:  exp.Status,
		Winner:  exp.Winner,
		Results: make(map[string]*VariantResult),
	}

	for _, split := range exp.TrafficSplit {
		vs := stats[split.Variant]
		if vs == nil {
			res.Results[split.Variant] = &VariantResult{Model: split.Model}
			continue
		}
		res.Results[split.Variant] = &VariantResult{
			Model:             split.Model,
			Samples:           vs.Samples,
			LatencyP95Ms:      vs.LatencyP95Ms,
			AvgCostPerRequest: vs.AvgCostPerReq,
			ErrorRate:         vs.ErrorRate,
		}
	}

	if len(exp.TrafficSplit) < 2 {
		return res
	}

	controlName := exp.TrafficSplit[0].Variant
	treatName := exp.TrafficSplit[1].Variant
	control := stats[controlName]
	treat := stats[treatName]

	// Require at least 30 samples per variant for meaningful statistics.
	if control == nil || treat == nil || control.Samples < 30 || treat.Samples < 30 {
		return res
	}

	alpha := 1.0 - exp.ConfidenceLevel
	res.Significance = make(map[string]*MetricSignificance)

	latP, latImp := tTestImprovement(control.AvgLatencyMs, treat.AvgLatencyMs, control.Samples, treat.Samples)
	res.Significance["latency"] = &MetricSignificance{
		PValue:         latP,
		Significant:    latP < alpha,
		ImprovementPct: latImp,
	}

	costP, costImp := tTestImprovement(control.AvgCostPerReq, treat.AvgCostPerReq, control.Samples, treat.Samples)
	res.Significance["cost"] = &MetricSignificance{
		PValue:         costP,
		Significant:    costP < alpha,
		ImprovementPct: costImp,
	}

	errP, errImp := zTestImprovement(control.ErrorRate, treat.ErrorRate, control.Samples, treat.Samples)
	res.Significance["error_rate"] = &MetricSignificance{
		PValue:         errP,
		Significant:    errP < alpha,
		ImprovementPct: errImp,
	}

	// Determine winner when all significant metrics favour treatment (lower is better).
	sigCount, treatBetter := 0, 0
	for _, sig := range res.Significance {
		if sig.Significant {
			sigCount++
			if sig.ImprovementPct < 0 {
				treatBetter++
			}
		}
	}
	if sigCount > 0 && treatBetter == sigCount {
		res.Winner = treatName
		t := res.Results[treatName]
		c := res.Results[controlName]
		costDiff := 0.0
		if c.AvgCostPerRequest != 0 {
			costDiff = (c.AvgCostPerRequest - t.AvgCostPerRequest) / c.AvgCostPerRequest * 100
		}
		latDiff := 0.0
		if c.LatencyP95Ms != 0 {
			latDiff = (c.LatencyP95Ms - t.LatencyP95Ms) / c.LatencyP95Ms * 100
		}
		res.Recommendation = fmt.Sprintf(
			"Switch to %s (%s): %.1f%% cost reduction, %.1f%% latency improvement, statistically significant.",
			treatName, t.Model, costDiff, latDiff,
		)
	} else if sigCount > 0 && treatBetter == 0 {
		res.Winner = controlName
		res.Recommendation = fmt.Sprintf(
			"Keep %s (%s): treatment showed no significant improvement.",
			controlName, res.Results[controlName].Model,
		)
	}

	return res
}

// tTestImprovement returns (p-value, improvementPct) using a normal approximation
// of Welch's t-test. improvementPct < 0 means treatment is better (lower value).
func tTestImprovement(controlMean, treatMean float64, n1, n2 int) (pValue, improvementPct float64) {
	if controlMean == 0 {
		return 1.0, 0
	}
	improvementPct = (treatMean - controlMean) / controlMean * 100

	// Approximate stddev as 20% of mean (no stored stddev available).
	sd1 := math.Max(controlMean*0.2, 1e-9)
	sd2 := math.Max(treatMean*0.2, 1e-9)

	se := math.Sqrt(sd1*sd1/float64(n1) + sd2*sd2/float64(n2))
	if se == 0 {
		return 1.0, improvementPct
	}
	z := math.Abs(controlMean-treatMean) / se
	return normalTailP(z), improvementPct
}

// zTestImprovement tests two proportions (e.g. error rates).
func zTestImprovement(p1, p2 float64, n1, n2 int) (pValue, improvementPct float64) {
	if p1 == 0 && p2 == 0 {
		return 1.0, 0
	}
	if p1 == 0 {
		return 1.0, 0
	}
	improvementPct = (p2 - p1) / p1 * 100

	pPool := (p1*float64(n1) + p2*float64(n2)) / float64(n1+n2)
	if pPool <= 0 || pPool >= 1 {
		return 1.0, improvementPct
	}
	se := math.Sqrt(pPool * (1 - pPool) * (1/float64(n1) + 1/float64(n2)))
	if se == 0 {
		return 1.0, improvementPct
	}
	z := math.Abs(p1-p2) / se
	return normalTailP(z), improvementPct
}

// normalTailP returns the two-tailed p-value for a z-statistic.
// Uses the complementary error function approximation (Abramowitz & Stegun 7.1.26).
// erfc(x) = (a1*t + ... + a5*t^5) * exp(-x^2), t = 1/(1+0.3275911*x)
// and the identity: 2*(1-Phi(z)) = erfc(z/sqrt(2)).
func normalTailP(z float64) float64 {
	x := z / math.Sqrt2
	t := 1.0 / (1.0 + 0.3275911*x)
	poly := t * (0.254829592 + t*(-0.284496736+t*(1.421413741+t*(-1.453152027+t*1.061405429))))
	return poly * math.Exp(-x*x)
}

// P95 returns the 95th-percentile of vals.
func P95(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	s := make([]float64, len(vals))
	copy(s, vals)
	sort.Float64s(s)
	idx := int(math.Ceil(0.95*float64(len(s)))) - 1
	if idx < 0 {
		idx = 0
	}
	return s[idx]
}
