package align

import (
	"math"
	"sort"

	"depthalign/internal/domain"
)

const (
	SourceStep = 0.25
	TargetStep = 0.1
)

func DefaultConfig() domain.SolveConfig {
	return domain.SolveConfig{MinStretch: 0.75, MaxStretch: 1.20, SoftWeight: 2}
}

type edgeCost struct {
	data    float64
	stretch float64
	soft    float64
	noData  bool
}

type stateCost struct {
	data    float64
	stretch float64
	soft    float64
}

type weights struct {
	data    float64
	stretch float64
	soft    float64
}

type candidateSpec struct {
	id   string
	name string
	w    weights
}

func Solve(data domain.Dataset, cfg domain.SolveConfig) domain.Result {
	if cfg.MinStretch <= 0 {
		cfg = DefaultConfig()
	}
	if cfg.MaxStretch < cfg.MinStretch {
		cfg.MaxStretch = cfg.MinStretch
	}
	result := domain.Result{
		Config:    cfg,
		Conflicts: append(crossingChains(data.Markers), stretchConflicts(data.Markers, cfg)...),
		Warnings:  markerWarnings(data.Markers, cfg),
	}
	if len(data.Curves) == 0 {
		result.Conflicts = append(result.Conflicts, domain.Conflict{Kind: "no_curves", Message: "没有可对齐的测井曲线"})
		return result
	}
	curves := prepareCurves(data.Curves)
	x0, x1 := curves[0].aMin, curves[0].aMax
	y0, y1 := curves[0].bMin, curves[0].bMax
	result.NoDataSegments = collectSourceGaps(curves)
	if len(result.Conflicts) > 0 {
		return result
	}

	if conflicts := datasetConflicts(data, sourceTargetBounds{aMin: x0, aMax: x1, bMin: y0, bMax: y1}); len(conflicts) > 0 {
		result.Conflicts = append(result.Conflicts, conflicts...)
		return result
	}
	nx := int(math.Round((x1-x0)/SourceStep)) + 1
	ny := int(math.Round((y1-y0)/TargetStep)) + 1
	result.Conflicts = append(result.Conflicts, gridStretchConflicts(data.Markers, x0, y0, x1, y1, nx, ny, cfg)...)
	if len(result.Conflicts) > 0 {
		return result
	}
	hardLayers := buildHardLayers(data.Markers, x0, y0, ny)
	softLayers := buildSoftLayers(data.Markers, x0, x1, y0, ny)

	specs := []candidateSpec{
		{id: "balanced", name: "均衡路径", w: weights{data: 1, stretch: 0.25, soft: cfg.SoftWeight}},
		{id: "shape", name: "曲线形状优先", w: weights{data: 1.6, stretch: 0.08, soft: 1}},
		{id: "smooth", name: "平滑伸缩优先", w: weights{data: 0.7, stretch: 0.7, soft: 1}},
	}
	for _, spec := range specs {
		candidate, err := dynamicProgram(data.Curves, curves, hardLayers, softLayers, x0, y0, nx, ny, cfg, spec)
		if err != nil {
			result.Conflicts = append(result.Conflicts, domain.Conflict{Kind: "no_path", Message: err.Error()})
			return result
		}
		result.Candidates = append(result.Candidates, candidate)
	}
	sort.SliceStable(result.Candidates, func(i, j int) bool {
		return canonicalScore(result.Candidates[i].Costs) < canonicalScore(result.Candidates[j].Costs)
	})
	result.OK = true
	result.SelectedID = result.Candidates[0].ID
	return result
}

func canonicalScore(c domain.CostBreakdown) float64 {
	return c.Data + 0.25*c.Stretch + 2*c.Soft
}

func collectSourceGaps(curves []preparedCurve) []domain.Interval {
	seen := map[domain.Interval]bool{}
	var out []domain.Interval
	for _, c := range curves {
		for _, g := range c.aGaps {
			if !seen[g] {
				seen[g] = true
				out = append(out, g)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].From < out[j].From })
	return out
}

func buildHardLayers(markers []domain.Marker, x0, y0 float64, ny int) map[int]int {
	out := make(map[int]int)
	for _, m := range markers {
		if !m.Active || m.Kind != "hard" {
			continue
		}
		ix := int(math.Round((m.PassAFrom - x0) / SourceStep))
		iy := int(math.Round((m.PassBTo - y0) / TargetStep))
		if ix >= 0 && iy >= 0 && iy < ny {
			out[ix] = iy
		}
	}
	return out
}

func buildSoftLayers(markers []domain.Marker, x0, x1, y0 float64, ny int) map[int]int {
	out := make(map[int]int)
	for _, m := range markers {
		if !m.Active || m.Kind != "soft" {
			continue
		}
		ix := int(math.Round((m.PassAFrom - x0) / SourceStep))
		iy := int(math.Round((m.PassBTo - y0) / TargetStep))
		if ix > 0 && ix < int(math.Round((x1-x0)/SourceStep)) && iy >= 0 && iy < ny {
			out[ix] = iy
		}
	}
	return out
}
