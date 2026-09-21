package align

import (
	"math"
	"sort"
)

type preparedCurve struct {
	key     string
	minA    float64
	maxA    float64
	minB    float64
	maxB    float64
	depthsA []float64
	depthsB []float64
	valuesA []float64
	valuesB []float64
}

type node struct {
	left  float64
	right float64
	band  int
	soft  map[string]float64
	hard  bool
}

type route struct {
	cost       Costs
	point      Point
	points     []Point
	first      int
	parentRank int
	signature  string
	prevLeft   float64
	prevRight  float64
}

type edgeInfo struct {
	similarity float64
	regularity float64
	kind       string
	gapM       float64
	valid      bool
}

func Solve(data RunData, controls []Control, proposals []Control, settings Settings) SolveResult {
	if settings.MaxCandidates <= 0 {
		settings = DefaultSettings()
	}
	active := ActiveControls(controls)
	conflicts := AnalyzeConflicts(active, proposals, settings)
	conflicts = append(conflicts, controlGapConflicts(active, data.GapsA, data.GapsB)...)
	for _, conflict := range conflicts {
		if conflict.Severity == "blocking" {
			if conflict.Code == "hard_stretch_violation" {
				return SolveResult{
					OK:                 false,
					Settings:           settings,
					Conflicts:          conflicts,
					BoundaryViolations: filterStretchViolations(conflicts),
					NoDataSegments:     noDataSegments(data),
					Message:            "存在阻断性锦标冲突，已保留候选路径生成前状态。",
				}
			}
			return SolveResult{
				OK:             false,
				Settings:       settings,
				Conflicts:      conflicts,
				NoDataSegments: noDataSegments(data),
				Message:        "存在阻断性锦标冲突，已保留候选路径生成前状态。",
			}
		}
	}

	curves := prepareCurves(data.Curves)
	nodes := buildNodes(data, active, curves)
	adjacency := buildAdjacency(data, nodes, active, curves, settings)
	routes := runDP(nodes, adjacency, active, settings)
	if len(routes) == 0 {
		conflicts = append(conflicts, Conflict{
			Severity: "blocking",
			Code:     "no_feasible_path",
			Message:  "伸缩率或空缺约束下不存在可行路径。",
		})
		return SolveResult{
			OK:             false,
			Settings:       settings,
			Conflicts:      conflicts,
			NoDataSegments: noDataSegments(data),
			Message:        "无可行单调路径。",
		}
	}

	candidates := make([]Candidate, 0, len(routes))
	for index, current := range routes {
		candidate := buildCandidate(index+1, current, nodes, active, curves, data, settings)
		candidates = append(candidates, candidate)
	}
	best := candidates[0]
	for index := range candidates {
		mad, maxDiff := compareCandidates(best.Points, candidates[index].Points)
		candidates[index].DiffFromBestMAD = mad
		candidates[index].DiffFromBestMax = maxDiff
	}

	return SolveResult{
		OK:             true,
		Settings:       settings,
		Candidates:     candidates,
		Conflicts:      AnalyzeConflicts(active, proposals, settings),
		NoDataSegments: noDataSegments(data),
		Message:        "求解完成。",
	}
}

func filterStretchViolations(conflicts []Conflict) []Conflict {
	out := []Conflict{}
	for _, conflict := range conflicts {
		if conflict.Code == "hard_stretch_violation" {
			out = append(out, conflict)
		}
	}
	return out
}

func prepareCurves(curves []Curve) []preparedCurve {
	out := make([]preparedCurve, 0, len(curves))
	for _, curve := range curves {
		prepared := preparedCurve{key: curve.Key}
		prepared.depthsA, prepared.valuesA, prepared.minA, prepared.maxA = flattenSamples(curve.SamplesA)
		prepared.depthsB, prepared.valuesB, prepared.minB, prepared.maxB = flattenSamples(curve.SamplesB)
		out = append(out, prepared)
	}
	return out
}

func flattenSamples(samples []Sample) ([]float64, []float64, float64, float64) {
	sorted := append([]Sample(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Depth < sorted[j].Depth })
	depths := make([]float64, len(sorted))
	values := make([]float64, len(sorted))
	minValue := math.Inf(1)
	maxValue := math.Inf(-1)
	for index, sample := range sorted {
		depths[index] = sample.Depth
		values[index] = sample.Value
		minValue = math.Min(minValue, sample.Value)
		maxValue = math.Max(maxValue, sample.Value)
	}
	return depths, values, minValue, maxValue
}

func buildNodes(data RunData, controls []Control, curves []preparedCurve) []node {
	seen := map[[2]float64]int{}
	var nodes []node
	add := func(left, right float64) {
		key := [2]float64{roundDepth(left), roundDepth(right)}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = len(nodes)
		nodes = append(nodes, node{left: left, right: right, band: bandIndex(data.GapsA, left), soft: map[string]float64{}})
	}

	leftDepths := []float64{data.DomainA.From, data.DomainA.To}
	rightDepths := []float64{data.DomainB.From, data.DomainB.To}
	for _, curve := range curves {
		leftDepths = append(leftDepths, curve.depthsA...)
		rightDepths = append(rightDepths, curve.depthsB...)
	}
	for _, control := range controls {
		leftDepths = append(leftDepths, control.LeftDepth)
		rightDepths = append(rightDepths, control.RightDepth)
	}
	leftDepths = uniqueSorted(filterBandDepths(data.GapsA, leftDepths))
	rightDepths = uniqueSorted(filterBandDepths(data.GapsB, rightDepths))

	for _, left := range leftDepths {
		for _, right := range rightDepths {
			if bandIndex(data.GapsA, left) == bandIndex(data.GapsB, right) {
				add(left, right)
			}
		}
	}

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].left == nodes[j].left {
			return nodes[i].right < nodes[j].right
		}
		return nodes[i].left < nodes[j].left
	})
	indexByDepth := map[[2]float64]int{}
	for index, current := range nodes {
		indexByDepth[[2]float64{roundDepth(current.left), roundDepth(current.right)}] = index
	}
	for _, control := range controls {
		index, exists := indexByDepth[[2]float64{roundDepth(control.LeftDepth), roundDepth(control.RightDepth)}]
		if !exists {
			continue
		}
		if control.Kind == "hard" {
			nodes[index].hard = true
		}
	}
	for _, control := range controls {
		if control.Kind != "soft" {
			continue
		}
		for index := range nodes {
			if math.Abs(nodes[index].left-control.LeftDepth) <= epsilon {
				nodes[index].soft[control.ID] = control.RightDepth
			}
		}
	}
	return nodes
}

func buildAdjacency(data RunData, nodes []node, controls []Control, curves []preparedCurve, settings Settings) [][]routeEdge {
	edges := make([][]routeEdge, len(nodes))
	leftObservations := uniqueSorted(append(append([]float64{data.DomainA.From, data.DomainA.To}, allLeftDepths(curves)...), controlDepths(controls, true)...))
	rightObservations := uniqueSorted(append(append([]float64{data.DomainB.From, data.DomainB.To}, allRightDepths(curves)...), controlDepths(controls, false)...))
	bridgeEndpoints := gapBridgeEndpoints(data.GapsA, data.GapsB, leftObservations, rightObservations)
	startByLeft := map[float64][]int{}
	for index, current := range nodes {
		startByLeft[roundDepth(current.left)] = append(startByLeft[roundDepth(current.left)], index)
	}
	leftKeys := make([]float64, 0, len(startByLeft))
	for left := range startByLeft {
		leftKeys = append(leftKeys, left)
	}
	sort.Float64s(leftKeys)

	for i, source := range nodes {
		for _, leftKey := range leftKeys {
			left := leftKey
			if left <= source.left+epsilon {
				continue
			}
			depthDelta := left - source.left
			if depthDelta > settings.SimilarityWindowM {
				if _, isBridge := bridgeEndpoints[endpointKey(source.left, left, source.right, 0)]; !isBridge {
					continue
				}
			}
			for _, j := range startByLeft[leftKey] {
				target := nodes[j]
				if target.right <= source.right+epsilon {
					continue
				}
				bridge, bridgeExists := bridgeEndpoints[endpointKey(source.left, target.left, source.right, target.right)]
				if target.band != source.band && !(bridgeExists && target.band == source.band+1) {
					continue
				}
				rightDelta := target.right - source.right
				if depthDelta <= epsilon || rightDelta <= epsilon {
					continue
				}
				stretch := rightDelta / depthDelta
				if stretch < settings.MinStretch-epsilon || stretch > settings.MaxStretch+epsilon {
					continue
				}
				if violatesHard(source, target, controls) {
					continue
				}
				var info edgeInfo
				if bridgeExists {
					info = edgeInfo{kind: "gap_bridge", gapM: bridge.gapM, regularity: regularityCost(depthDelta, stretch, settings), valid: true}
				} else {
					info = evaluateEdge(source, target, curves, settings)
				}
				if !info.valid {
					continue
				}
				edges[i] = append(edges[i], routeEdge{to: j, info: info})
			}
		}
		sort.Slice(edges[i], func(a, b int) bool { return edges[i][a].to < edges[i][b].to })
	}
	return edges
}

type routeEdge struct {
	to   int
	info edgeInfo
}

type bridgeKey struct {
	fromLeft  float64
	toLeft    float64
	fromRight float64
	toRight   float64
}

type bridgeSpec struct {
	gapM float64
}

func controlGapConflicts(controls []Control, gapsA, gapsB []Interval) []Conflict {
	conflicts := []Conflict{}
	for _, control := range controls {
		if inAnyInterval(control.LeftDepth, gapsA) || inAnyInterval(control.RightDepth, gapsB) {
			conflicts = append(conflicts, Conflict{
				Severity:   "blocking",
				Code:       "control_inside_no_data",
				Message:    "控制点落在无数据区段内，不能把无数据样本解释为零值对齐。",
				ControlIDs: []string{control.ID},
			})
		}
	}
	return conflicts
}
