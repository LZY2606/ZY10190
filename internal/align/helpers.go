package align

import (
	"math"
	"sort"
)

func roundDepth(value float64) float64 {
	return math.Round(value*1000000) / 1000000
}

func uniqueSorted(values []float64) []float64 {
	seen := map[float64]struct{}{}
	out := make([]float64, 0, len(values))
	for _, value := range values {
		key := roundDepth(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	sort.Float64s(out)
	return out
}

func filterBandDepths(gaps []Interval, values []float64) []float64 {
	out := values[:0]
	for _, value := range values {
		if !inAnyInterval(value, gaps) {
			out = append(out, value)
		}
	}
	return out
}

func bandIndex(gaps []Interval, depth float64) int {
	band := 0
	for _, gap := range gaps {
		if depth >= gap.To-epsilon {
			band++
			continue
		}
		return band
	}
	return band
}

func allLeftDepths(curves []preparedCurve) []float64 {
	var values []float64
	for _, curve := range curves {
		values = append(values, curve.depthsA...)
	}
	return values
}

func allRightDepths(curves []preparedCurve) []float64 {
	var values []float64
	for _, curve := range curves {
		values = append(values, curve.depthsB...)
	}
	return values
}

func controlDepths(controls []Control, left bool) []float64 {
	values := make([]float64, 0, len(controls))
	for _, control := range controls {
		if left {
			values = append(values, control.LeftDepth)
		} else {
			values = append(values, control.RightDepth)
		}
	}
	return values
}

func endpointKey(fromLeft, toLeft, fromRight, toRight float64) bridgeKey {
	return bridgeKey{
		fromLeft:  roundDepth(fromLeft),
		toLeft:    roundDepth(toLeft),
		fromRight: roundDepth(fromRight),
		toRight:   roundDepth(toRight),
	}
}

func nearestBefore(values []float64, target float64) (float64, bool) {
	index := sort.SearchFloat64s(values, target+epsilon) - 1
	if index < 0 {
		return 0, false
	}
	return values[index], true
}

func nearestAfter(values []float64, target float64) (float64, bool) {
	index := sort.SearchFloat64s(values, target-epsilon)
	if index >= len(values) {
		return 0, false
	}
	return values[index], true
}

func gapBridgeEndpoints(gapsA, gapsB []Interval, leftValues, rightValues []float64) map[bridgeKey]bridgeSpec {
	out := map[bridgeKey]bridgeSpec{}
	for index := range gapsA {
		gapA := gapsA[index]
		gapB := gapsB[index]
		fromLeft, okA1 := nearestBefore(leftValues, gapA.From)
		toLeft, okA2 := nearestAfter(leftValues, gapA.To)
		fromRight, okB1 := nearestBefore(rightValues, gapB.From)
		toRight, okB2 := nearestAfter(rightValues, gapB.To)
		if okA1 && okA2 && okB1 && okB2 {
			out[endpointKey(fromLeft, toLeft, fromRight, toRight)] = bridgeSpec{gapM: gapA.To - gapA.From + gapB.To - gapB.From}
		}
	}
	return out
}

func violatesHard(source, target node, controls []Control) bool {
	for _, control := range controls {
		if control.Kind != "hard" {
			continue
		}
		atSourceLeft := math.Abs(source.left-control.LeftDepth) <= epsilon
		atTargetLeft := math.Abs(target.left-control.LeftDepth) <= epsilon
		if atSourceLeft && math.Abs(source.right-control.RightDepth) > epsilon {
			return true
		}
		if atTargetLeft && math.Abs(target.right-control.RightDepth) > epsilon {
			return true
		}
		if control.LeftDepth > source.left+epsilon && control.LeftDepth < target.left-epsilon {
			return true
		}
	}
	return false
}

func regularityCost(depthDelta, stretch float64, settings Settings) float64 {
	return settings.RegularityWeight * math.Pow(stretch-1, 2) * depthDelta
}

func evaluateEdge(source, target node, curves []preparedCurve, settings Settings) edgeInfo {
	info := edgeInfo{kind: "data", valid: true}
	depthDelta := target.left - source.left
	stretch := (target.right - source.right) / depthDelta
	info.regularity = regularityCost(depthDelta, stretch, settings)

	for _, curve := range curves {
		cost, count := curveEdgeCost(curve, source, target)
		if count == 0 {
			info.valid = false
			return info
		}
		info.similarity += cost
	}
	info.similarity /= float64(len(curves))
	return info
}

func curveEdgeCost(curve preparedCurve, source, target node) (float64, int) {
	start := sort.SearchFloat64s(curve.depthsA, source.left-epsilon)
	end := sort.SearchFloat64s(curve.depthsA, target.left+epsilon)
	if start < 0 || end <= start || end > len(curve.depthsA) {
		return 0, 0
	}
	var total float64
	var previousDepth, previousValue float64
	count := 0
	for index := start; index < end; index++ {
		left := curve.depthsA[index]
		right := source.right + (target.right-source.right)*((left-source.left)/(target.left-source.left))
		bValue, ok := interpolate(curve.depthsB, curve.valuesB, right)
		if !ok {
			return 0, 0
		}
		normalizedA := normalize(curve.valuesA[index], curve.minA, curve.maxA)
		normalizedB := normalize(bValue, curve.minB, curve.maxB)
		diff := normalizedA - normalizedB
		value := diff * diff
		if count > 0 {
			total += 0.5 * (previousValue + value) * (left - previousDepth)
		}
		previousDepth = left
		previousValue = value
		count++
	}
	return total, count
}

func interpolate(depths, values []float64, target float64) (float64, bool) {
	index := sort.SearchFloat64s(depths, target)
	if index < len(depths) && math.Abs(depths[index]-target) <= epsilon {
		return values[index], true
	}
	if index == 0 || index >= len(depths) {
		return 0, false
	}
	leftDepth := depths[index-1]
	rightDepth := depths[index]
	if target < leftDepth-epsilon || target > rightDepth+epsilon {
		return 0, false
	}
	fraction := (target - leftDepth) / (rightDepth - leftDepth)
	return values[index-1] + fraction*(values[index]-values[index-1]), true
}

func normalize(value, minValue, maxValue float64) float64 {
	if math.Abs(maxValue-minValue) <= epsilon {
		return 0
	}
	return (value - minValue) / (maxValue - minValue)
}

func noDataSegments(data RunData) []Segment {
	segments := make([]Segment, 0, len(data.GapsA))
	for index := range data.GapsA {
		gapA := data.GapsA[index]
		gapB := data.GapsB[index]
		from := Point{Left: gapA.From, Right: gapB.From, Band: index}
		to := Point{Left: gapA.To, Right: gapB.To, Band: index + 1}
		stretch := (to.Right - from.Right) / (to.Left - from.Left)
		segments = append(segments, Segment{From: from, To: to, Stretch: stretch, Kind: "no_data"})
	}
	return segments
}
