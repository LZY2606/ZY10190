package align

import (
	"fmt"
	"math"

	"depthalign/internal/domain"
)

type midValue struct {
	value float64
	valid bool
}

type midpointSet struct {
	a [][]midValue
	b [][]midValue
}

func dynamicProgram(
	rawCurves map[string]domain.Curve,
	curves []preparedCurve,
	hardLayers, softLayers map[int]int,
	x0, y0 float64,
	nx, ny int,
	cfg domain.SolveConfig,
	spec candidateSpec,
) (domain.Candidate, error) {
	mids := buildMidpoints(curves, nx, ny, x0, y0)
	inf := math.Inf(1)
	prev := make([]stateCost, ny)
	cur := make([]stateCost, ny)
	for j := range prev {
		prev[j].data = inf
		cur[j].data = inf
	}
	prev[0] = stateCost{}
	parents := make([][]int, nx)
	for i := range parents {
		parents[i] = make([]int, ny)
	}
	minD := int(math.Ceil(cfg.MinStretch*SourceStep/TargetStep - 1e-9))
	maxD := int(math.Floor(cfg.MaxStretch*SourceStep/TargetStep + 1e-9))
	if minD < 1 || maxD < minD {
		return domain.Candidate{}, fmt.Errorf("伸缩率配置在当前网格上没有可用步长")
	}

	for i := 1; i < nx; i++ {
		for k := range cur {
			cur[k].data = inf
		}
		targets := kRange(ny, hardLayers, i, i == nx-1)
		for _, k := range targets {
			bestJ := -1
			best := stateCost{data: inf}
			for d := minD; d <= maxD; d++ {
				j := k - d
				if j < 0 || j >= ny || math.IsInf(prev[j].data, 1) {
					continue
				}
				edgeX0 := x0 + float64(i-1)*SourceStep
				edgeX1 := x0 + float64(i)*SourceStep
				edgeY0 := y0 + float64(j)*TargetStep
				edgeY1 := y0 + float64(k)*TargetStep
				if !gapsAligned(edgeX0, edgeX1, edgeY0, edgeY1, curves) {
					continue
				}
				ec := transitionCost(mids, curves, softLayers, i, j, k)
				candidate := stateCost{
					data:    prev[j].data + ec.data,
					stretch: prev[j].stretch + ec.stretch,
					soft:    prev[j].soft + ec.soft,
				}
				score := spec.w.data*candidate.data + spec.w.stretch*candidate.stretch + spec.w.soft*candidate.soft
				if bestJ == -1 || score < spec.w.data*best.data+spec.w.stretch*best.stretch+spec.w.soft*best.soft {
					best = candidate
					bestJ = j
				}
			}
			if bestJ >= 0 {
				cur[k] = best
				parents[i][k] = bestJ
			}
		}
		copy(prev, cur)
	}

	if math.IsInf(prev[ny-1].data, 1) {
		return domain.Candidate{}, fmt.Errorf("没有满足单调、非压缩和伸缩范围的候选路径")
	}

	path := make([]domain.Point, nx)
	k := ny - 1
	for i := nx - 1; i >= 0; i-- {
		path[i] = domain.Point{From: round2(x0 + float64(i)*SourceStep), To: round1(y0 + float64(k)*TargetStep)}
		if i > 0 {
			k = parents[i][k]
		}
	}
	if k != 0 {
		return domain.Candidate{}, fmt.Errorf("候选路径未连接井段起点")
	}

	stretchMin, stretchMax, violations := segmentIssues(path, cfg)
	nodata := nodataIntervals(path, mids, curves)
	costs := domain.CostBreakdown{
		Data:    prev[ny-1].data,
		Stretch: prev[ny-1].stretch,
		Soft:    prev[ny-1].soft,
	}
	costs.Total = spec.w.data*costs.Data + spec.w.stretch*costs.Stretch + spec.w.soft*costs.Soft
	return domain.Candidate{
		ID:          spec.id,
		Name:        spec.name,
		Points:      path,
		Costs:       costs,
		StretchMin:  stretchMin,
		StretchMax:  stretchMax,
		Violations:  violations,
		NodataEdges: nodata,
		Diffs:       alignmentDifferences(rawCurves, path),
	}, nil
}

func kRange(ny int, hard map[int]int, layer int, isFinal bool) []int {
	if y, ok := hard[layer]; ok {
		return []int{y}
	}
	if isFinal {
		return []int{ny - 1}
	}
	out := make([]int, ny)
	for i := range out {
		out[i] = i
	}
	return out
}

func buildMidpoints(curves []preparedCurve, nx, ny int, x0, y0 float64) midpointSet {
	out := midpointSet{
		a: make([][]midValue, len(curves)),
		b: make([][]midValue, len(curves)),
	}
	for ci, c := range curves {
		out.a[ci] = make([]midValue, nx-1)
		for i := 0; i < nx-1; i++ {
			xm := x0 + (float64(i)+0.5)*SourceStep
			v, ok := valueAt(c.aDepths, c.aValues, xm, c.aGaps)
			out.a[ci][i] = midValue{value: (v - c.aMean) / c.aScale, valid: ok}
		}
		count := 2*(ny-1) - 1
		out.b[ci] = make([]midValue, count)
		for p := 0; p < count; p++ {
			ym := y0 + (float64(p)+1)*0.05
			v, ok := valueAt(c.bDepths, c.bValues, ym, c.bGaps)
			out.b[ci][p] = midValue{value: (v - c.bMean) / c.bScale, valid: ok}
		}
	}
	return out
}

func transitionCost(mids midpointSet, curves []preparedCurve, soft map[int]int, layer, j, k int) edgeCost {
	var dataSum float64
	var valid int
	p := j + k - 1
	for ci := range curves {
		av := mids.a[ci][layer-1]
		bv := mids.b[ci][p]
		if av.valid && bv.valid {
			d := av.value - bv.value
			dataSum += d * d
			valid++
		}
	}
	ec := edgeCost{noData: valid == 0}
	if valid > 0 {
		ec.data = dataSum / float64(valid)
	}
	stretch := float64(k-j) * TargetStep / SourceStep
	ec.stretch = (stretch - 1) * (stretch - 1)
	if target, ok := soft[layer]; ok {
		deviation := (float64(target-k) * TargetStep) / SourceStep
		ec.soft = deviation * deviation
	}
	return ec
}

func segmentIssues(path []domain.Point, cfg domain.SolveConfig) (float64, float64, []domain.SegmentIssue) {
	minS := math.Inf(1)
	maxS := math.Inf(-1)
	var issues []domain.SegmentIssue
	for i := 0; i+1 < len(path); i++ {
		dx := path[i+1].From - path[i].From
		dy := path[i+1].To - path[i].To
		if dx <= 0 {
			return minS, maxS, []domain.SegmentIssue{{From: path[i].From, To: path[i+1].From, Reason: "non_monotonic_source"}}
		}
		if dy <= 0 {
			issues = append(issues, domain.SegmentIssue{From: path[i].From, To: path[i+1].From, FromMapped: path[i].To, ToMapped: path[i+1].To, Stretch: 0, Reason: "non_positive_target"})
			continue
		}
		s := dy / dx
		minS = math.Min(minS, s)
		maxS = math.Max(maxS, s)
		if s < cfg.MinStretch-1e-9 || s > cfg.MaxStretch+1e-9 {
			issues = append(issues, domain.SegmentIssue{From: path[i].From, To: path[i+1].From, FromMapped: path[i].To, ToMapped: path[i+1].To, Stretch: s, Reason: "stretch_range"})
		}
	}
	return minS, maxS, issues
}

func nodataIntervals(path []domain.Point, mids midpointSet, curves []preparedCurve) []domain.Interval {
	var out []domain.Interval
	var start float64
	inGap := false
	for i := 1; i < len(path); i++ {
		valid := false
		p := int(math.Round((path[i].To+path[i-1].To-2*curves[0].bMin)/TargetStep)) - 1
		for ci := range curves {
			if mids.a[ci][i-1].valid && mids.b[ci][p].valid {
				valid = true
			}
		}
		if !valid && !inGap {
			start = path[i-1].From
			inGap = true
		}
		if valid && inGap {
			out = append(out, domain.Interval{From: start, To: path[i-1].From})
			inGap = false
		}
	}
	if inGap {
		out = append(out, domain.Interval{From: start, To: path[len(path)-1].From})
	}
	return out
}
