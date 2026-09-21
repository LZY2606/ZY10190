package align

import "math"

func buildCandidate(rank int, current route, nodes []node, controls []Control, curves []preparedCurve, data RunData, settings Settings) Candidate {
	candidate := Candidate{
		Rank:     rank,
		Points:   current.points,
		Costs:    current.cost,
		Segments: buildSegments(current.points, curves, settings),
	}
	candidate.SoftDeviations = buildSoftDeviations(current.points, controls, settings)
	candidate.Residuals = buildResiduals(current.points, curves)
	candidate.GapBridgeCount = len(data.GapsA)
	return candidate
}

func buildSegments(points []Point, curves []preparedCurve, settings Settings) []Segment {
	segments := make([]Segment, 0, len(points)-1)
	for index := 0; index+1 < len(points); index++ {
		from := points[index]
		to := points[index+1]
		depthDelta := to.Left - from.Left
		rightDelta := to.Right - from.Right
		stretch := rightDelta / depthDelta
		kind := "data"
		if to.Band > from.Band {
			kind = "gap_bridge"
		}
		segment := Segment{From: from, To: to, Stretch: stretch, Kind: kind}
		if kind == "data" {
			source := node{left: from.Left, right: from.Right, band: from.Band}
			target := node{left: to.Left, right: to.Right, band: to.Band}
			info := evaluateEdge(source, target, curves, settings)
			segment.Similarity = info.similarity
			segment.Regularity = info.regularity
		} else {
			segment.Regularity = regularityCost(depthDelta, stretch, settings)
			segment.Warning = "无数据桥接：不计算跨空缺相似性"
		}
		if stretch < settings.MinStretch-epsilon || stretch > settings.MaxStretch+epsilon {
			segment.Warning = "伸缩率超出配置范围"
		}
		segments = append(segments, segment)
	}
	return segments
}

func buildSoftDeviations(points []Point, controls []Control, settings Settings) []SoftDeviation {
	out := []SoftDeviation{}
	for _, control := range controls {
		if control.Kind != "soft" {
			continue
		}
		actual, ok := interpolateRight(points, control.LeftDepth)
		if !ok {
			continue
		}
		deviation := actual - control.RightDepth
		if math.Abs(deviation) <= epsilon {
			out = append(out, SoftDeviation{ControlID: control.ID, Expected: control.RightDepth, Actual: actual})
			continue
		}
		weight := control.Penalty
		if weight <= 0 {
			weight = settings.SoftWeight
		}
		out = append(out, SoftDeviation{
			ControlID: control.ID, Expected: control.RightDepth, Actual: actual,
			DeviationM: deviation, Cost: weight * deviation * deviation,
		})
	}
	return out
}

func buildResiduals(points []Point, curves []preparedCurve) []Residual {
	out := make([]Residual, 0, len(curves))
	for _, curve := range curves {
		residual := Residual{CurveKey: curve.key}
		for index, left := range curve.depthsA {
			right, ok := interpolateRight(points, left)
			if !ok {
				continue
			}
			bValue, ok := interpolate(curve.depthsB, curve.valuesB, right)
			if !ok {
				continue
			}
			aValue := normalize(curve.valuesA[index], curve.minA, curve.maxA)
			bNormalized := normalize(bValue, curve.minB, curve.maxB)
			diff := aValue - bNormalized
			residual.SSE += diff * diff
			residual.SAE += math.Abs(diff)
			residual.MaxAbs = math.Max(residual.MaxAbs, math.Abs(diff))
			residual.Count++
		}
		out = append(out, residual)
	}
	return out
}

func interpolateRight(points []Point, left float64) (float64, bool) {
	if len(points) == 0 || left < points[0].Left-epsilon || left > points[len(points)-1].Left+epsilon {
		return 0, false
	}
	for index := 0; index+1 < len(points); index++ {
		from := points[index]
		to := points[index+1]
		if left >= from.Left-epsilon && left <= to.Left+epsilon {
			if math.Abs(to.Left-from.Left) <= epsilon {
				return from.Right, true
			}
			return from.Right + (to.Right-from.Right)*((left-from.Left)/(to.Left-from.Left)), true
		}
	}
	return points[len(points)-1].Right, true
}

func compareCandidates(best, other []Point) (float64, float64) {
	var sum, max float64
	count := 0
	for _, point := range other {
		bestRight, ok := interpolateRight(best, point.Left)
		if !ok {
			continue
		}
		diff := math.Abs(bestRight - point.Right)
		sum += diff
		max = math.Max(max, diff)
		count++
	}
	if count == 0 {
		return 0, 0
	}
	return sum / float64(count), max
}
