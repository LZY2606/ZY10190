package align

import (
	"math"
	"sort"
	"strconv"
)

func runDP(nodes []node, edges [][]routeEdge, controls []Control, settings Settings) []route {
	k := settings.MaxCandidates*8 + 12
	routesAt := make([][]route, len(nodes))
	startIndex := -1
	endIndex := -1
	minLeft := math.Inf(1)
	maxLeft := math.Inf(-1)
	minRight := math.Inf(1)
	maxRight := math.Inf(-1)
	for _, current := range nodes {
		minLeft = math.Min(minLeft, current.left)
		maxLeft = math.Max(maxLeft, current.left)
		minRight = math.Min(minRight, current.right)
		maxRight = math.Max(maxRight, current.right)
	}
	for index, current := range nodes {
		if current.left == minLeft && current.right == minRight {
			startIndex = index
		}
		if current.left == maxLeft && current.right == maxRight {
			endIndex = index
		}
	}
	if startIndex < 0 || endIndex < 0 {
		return nil
	}

	routesAt[startIndex] = []route{{
		cost:       Costs{},
		point:      Point{Left: nodes[startIndex].left, Right: nodes[startIndex].right, Band: nodes[startIndex].band},
		first:      -1,
		parentRank: -1,
		signature:  strconv.Itoa(startIndex),
	}}

	for sourceIndex, source := range nodes {
		best := routesAt[sourceIndex]
		if len(best) == 0 {
			continue
		}
		for _, edge := range edges[sourceIndex] {
			target := nodes[edge.to]
			for parentRank, parent := range best {
				candidate := parent.cost
				candidate.Similarity += edge.info.similarity
				candidate.Regularity += edge.info.regularity
				candidate.GapBridgesM += edge.info.gapM
				candidate.EvaluatedEdges++
				candidate.SoftPenalty += softCost(source, target, controls, settings)
				candidate.Total = candidate.Similarity + candidate.Regularity + candidate.SoftPenalty
				next := route{
					cost: candidate,
					point: Point{
						Left:     target.left,
						Right:    target.right,
						Band:     target.band,
						Required: target.hard || target.band != source.band,
					},
					first:      sourceIndex,
					parentRank: parentRank,
					prevLeft:   source.left,
					prevRight:  source.right,
				}
				corner := target.hard || target.band != source.band
				if !corner && parent.prevLeft != 0 {
					before := (source.right - parent.prevRight) / (source.left - parent.prevLeft)
					after := (target.right - source.right) / (target.left - source.left)
					corner = math.Abs(before-after) > epsilon
				}
				if corner {
					next.signature = parent.signature + ">" + strconv.Itoa(edge.to)
				} else {
					next.signature = parent.signature
				}
				routesAt[edge.to] = insertRoute(routesAt[edge.to], next, k)
			}
		}
	}

	finals := routesAt[endIndex]
	paths := make([]route, 0, len(finals))
	for _, current := range finals {
		paths = append(paths, reconstructRoute(current, routesAt))
	}
	sort.Slice(paths, func(i, j int) bool {
		if math.Abs(paths[i].cost.Total-paths[j].cost.Total) > epsilon {
			return paths[i].cost.Total < paths[j].cost.Total
		}
		return paths[i].cost.Similarity < paths[j].cost.Similarity
	})
	for index := range paths {
		paths[index].points = simplifyPoints(paths[index].points)
	}
	paths = uniqueRoutes(paths)
	geometricallyDistinct := make([]route, 0, settings.MaxCandidates)
	for _, current := range paths {
		duplicate := false
		for _, existing := range geometricallyDistinct {
			mad, maxDiff := compareCandidates(existing.points, current.points)
			if maxDiff <= epsilon && mad <= epsilon {
				duplicate = true
				break
			}
		}
		if !duplicate {
			geometricallyDistinct = append(geometricallyDistinct, current)
		}
	}
	paths = geometricallyDistinct
	if len(paths) > settings.MaxCandidates {
		paths = paths[:settings.MaxCandidates]
	}
	return paths
}

func simplifyPoints(points []Point) []Point {
	if len(points) < 3 {
		return points
	}
	out := []Point{points[0]}
	for index := 1; index < len(points)-1; index++ {
		if points[index].Required {
			out = append(out, points[index])
			continue
		}
		previous := out[len(out)-1]
		current := points[index]
		next := points[index+1]
		slopeBefore := (current.Right - previous.Right) / (current.Left - previous.Left)
		slopeAfter := (next.Right - current.Right) / (next.Left - current.Left)
		if math.Abs(slopeBefore-slopeAfter) > epsilon {
			out = append(out, current)
		}
	}
	out = append(out, points[len(points)-1])
	return out
}

func softCost(source, target node, controls []Control, settings Settings) float64 {
	var total float64
	for _, control := range controls {
		if control.Kind != "soft" || control.LeftDepth <= source.left+epsilon || control.LeftDepth > target.left+epsilon {
			continue
		}
		actual, ok := interpolateRight([]Point{{Left: source.left, Right: source.right}, {Left: target.left, Right: target.right}}, control.LeftDepth)
		if !ok || math.Abs(actual-control.RightDepth) <= epsilon {
			continue
		}
		weight := control.Penalty
		if weight <= 0 {
			weight = settings.SoftWeight
		}
		deviation := actual - control.RightDepth
		total += weight * deviation * deviation
	}
	return total
}

func insertRoute(routes []route, candidate route, k int) []route {
	for _, existing := range routes {
		if existing.signature == candidate.signature {
			return routes
		}
	}
	routes = append(routes, candidate)
	sort.Slice(routes, func(i, j int) bool {
		if math.Abs(routes[i].cost.Total-routes[j].cost.Total) > epsilon {
			return routes[i].cost.Total < routes[j].cost.Total
		}
		return routes[i].cost.Similarity < routes[j].cost.Similarity
	})
	if len(routes) > k {
		routes = routes[:k]
	}
	return routes
}

func routesEqual(a, b []Point) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if math.Abs(a[index].Left-b[index].Left) > epsilon || math.Abs(a[index].Right-b[index].Right) > epsilon {
			return false
		}
	}
	return true
}

func reconstructRoute(last route, routesAt [][]route) route {
	ordered := []Point{}
	current := last
	for {
		ordered = append(ordered, current.point)
		parentIndex := current.first
		if parentIndex < 0 {
			break
		}
		parent := routesAt[parentIndex][current.parentRank]
		current = parent
	}
	for left, right := 0, len(ordered)-1; left < right; left, right = left+1, right-1 {
		ordered[left], ordered[right] = ordered[right], ordered[left]
	}
	return route{cost: last.cost, point: last.point, points: ordered, signature: last.signature}
}

func uniqueRoutes(routes []route) []route {
	out := make([]route, 0, len(routes))
	for _, current := range routes {
		duplicate := false
		for _, existing := range out {
			if routesEqual(existing.points, current.points) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, current)
		}
	}
	return out
}
