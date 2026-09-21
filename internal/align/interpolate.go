package align

import (
	"math"
	"sort"

	"depthalign/internal/domain"
)

type preparedCurve struct {
	name    string
	aDepths []float64
	aValues []float64
	bDepths []float64
	bValues []float64
	aGaps   []domain.Interval
	bGaps   []domain.Interval
	aMin    float64
	aMax    float64
	bMin    float64
	bMax    float64
	aMean   float64
	aScale  float64
	bMean   float64
	bScale  float64
}

func prepareCurves(curves map[string]domain.Curve) []preparedCurve {
	out := make([]preparedCurve, 0)
	names := make(map[string]struct{})
	for _, c := range curves {
		names[c.Name] = struct{}{}
	}
	for name := range names {
		a := curves[name+":A"]
		b := curves[name+":B"]
		pc := preparedCurve{
			name:  name,
			aGaps: append([]domain.Interval(nil), a.Nodata...),
			bGaps: append([]domain.Interval(nil), b.Nodata...),
			aMin:  a.MinDepth,
			aMax:  a.MaxDepth,
			bMin:  b.MinDepth,
			bMax:  b.MaxDepth,
		}
		for _, s := range a.Samples {
			pc.aDepths = append(pc.aDepths, s.Depth)
			pc.aValues = append(pc.aValues, s.Value)
		}
		for _, s := range b.Samples {
			pc.bDepths = append(pc.bDepths, s.Depth)
			pc.bValues = append(pc.bValues, s.Value)
		}
		sort.Slice(pc.aGaps, func(i, j int) bool { return pc.aGaps[i].From < pc.aGaps[j].From })
		sort.Slice(pc.bGaps, func(i, j int) bool { return pc.bGaps[i].From < pc.bGaps[j].From })
		pc.aMean, pc.aScale = stats(pc.aValues)
		pc.bMean, pc.bScale = stats(pc.bValues)
		out = append(out, pc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

func stats(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 1
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))
	var sq float64
	for _, v := range values {
		d := v - mean
		sq += d * d
	}
	scale := math.Sqrt(sq / float64(len(values)))
	if scale < 1e-9 {
		scale = 1
	}
	return mean, scale
}

func valueAt(depths, values []float64, depth float64, gaps []domain.Interval) (float64, bool) {
	if len(depths) == 0 || depth < depths[0] || depth > depths[len(depths)-1] {
		return 0, false
	}
	if inAny(depth, gaps) {
		return 0, false
	}
	i := sort.SearchFloat64s(depths, depth)
	if i < len(depths) && math.Abs(depths[i]-depth) < 1e-9 {
		return values[i], true
	}
	i--
	if i < 0 || i+1 >= len(depths) {
		return 0, false
	}
	if inAny(depths[i], gaps) || inAny(depths[i+1], gaps) {
		return 0, false
	}
	if depths[i+1] <= depths[i] {
		return 0, false
	}
	t := (depth - depths[i]) / (depths[i+1] - depths[i])
	return values[i]*(1-t) + values[i+1]*t, true
}

func inAny(d float64, intervals []domain.Interval) bool {
	for _, in := range intervals {
		if d >= in.From-1e-9 && d <= in.To+1e-9 {
			return true
		}
	}
	return false
}

func overlap(a0, a1, b0, b1 float64) bool {
	return min(a1, b1) > max(a0, b0)+1e-9
}

func gapsAligned(x0, x1, y0, y1 float64, curves []preparedCurve) bool {
	xGap := overlapsGap(x0, x1, curves, true)
	yGap := overlapsGap(y0, y1, curves, false)
	return xGap == yGap
}

func overlapsGap(from, to float64, curves []preparedCurve, source bool) bool {
	for _, c := range curves {
		for _, g := range c.aGaps {
			if source && overlap(from, to, g.From, g.To) {
				return true
			}
		}
		for _, g := range c.bGaps {
			if !source && overlap(from, to, g.From, g.To) {
				return true
			}
		}
	}
	return false
}
