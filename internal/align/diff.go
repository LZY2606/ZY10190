package align

import (
	"math"
	"sort"

	"depthalign/internal/domain"
)

func alignmentDifferences(curves map[string]domain.Curve, path []domain.Point) []domain.CurveDiff {
	names := make(map[string]struct{})
	for _, c := range curves {
		names[c.Name] = struct{}{}
	}
	var ordered []string
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)

	var out []domain.CurveDiff
	for _, name := range ordered {
		a := curves[name+":A"]
		b := curves[name+":B"]
		_, aScale := stats(extractValues(a.Samples))
		diff := domain.CurveDiff{Name: name}
		var sq float64
		for x := path[0].From; x <= path[len(path)-1].From; x += 2 {
			y, ok := mapPath(path, x)
			if !ok {
				continue
			}
			av, aok := valueAt(extractDepth(a.Samples), extractValues(a.Samples), x, a.Nodata)
			bv, bok := valueAt(extractDepth(b.Samples), extractValues(b.Samples), y, b.Nodata)
			if !aok || !bok {
				continue
			}
			d := (av - bv) / aScale
			sq += d * d
			diff.Data = append(diff.Data, domain.DiffSample{From: round1(x), To: round1(y), AValue: av, BValue: bv, Diff: d})
		}
		if len(diff.Data) > 0 {
			diff.RMS = math.Sqrt(sq / float64(len(diff.Data)))
		}
		out = append(out, diff)
	}
	return out
}

func mapPath(path []domain.Point, x float64) (float64, bool) {
	i := sort.Search(len(path), func(i int) bool { return path[i].From >= x })
	if i < len(path) && math.Abs(path[i].From-x) < 1e-9 {
		return path[i].To, true
	}
	if i <= 0 || i >= len(path) {
		return 0, false
	}
	left := path[i-1]
	right := path[i]
	if right.From <= left.From || right.To <= left.To {
		return 0, false
	}
	t := (x - left.From) / (right.From - left.From)
	return left.To + t*(right.To-left.To), true
}

func extractDepth(samples []domain.Sample) []float64 {
	out := make([]float64, len(samples))
	for i, s := range samples {
		out[i] = s.Depth
	}
	return out
}

func extractValues(samples []domain.Sample) []float64 {
	out := make([]float64, len(samples))
	for i, s := range samples {
		out[i] = s.Value
	}
	return out
}
