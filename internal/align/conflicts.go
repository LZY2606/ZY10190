package align

import "math"

func AnalyzeConflicts(controls []Control, proposals []Control, settings Settings) []Conflict {
	conflicts := []Conflict{}
	hard := make([]Control, 0)
	soft := make([]Control, 0)
	for _, control := range controls {
		if control.Kind == "hard" {
			hard = append(hard, control)
		} else {
			soft = append(soft, control)
		}
	}

	for i := 0; i < len(hard); i++ {
		for j := i + 1; j < len(hard); j++ {
			left := hard[i]
			right := hard[j]
			if math.Abs(left.LeftDepth-right.LeftDepth) <= epsilon {
				conflicts = append(conflicts, Conflict{
					Severity:   "blocking",
					Code:       "hard_same_depth",
					Message:    "两个硬锦标共用左深但右深不同，非零深度段被压成一点。",
					ControlIDs: []string{left.ID, right.ID},
					Chain:      shortestConflictChain(hard, left, right),
					FromLeft:   left.LeftDepth,
					ToLeft:     right.LeftDepth,
					FromRight:  left.RightDepth,
					ToRight:    right.RightDepth,
				})
				continue
			}
			if left.LeftDepth < right.LeftDepth && left.RightDepth >= right.RightDepth-epsilon {
				conflicts = append(conflicts, Conflict{
					Severity:   "blocking",
					Code:       "hard_order_crossed",
					Message:    "两个硬锦标顺序交叉，映射不能同时通过二者。",
					ControlIDs: []string{left.ID, right.ID},
					Chain:      shortestConflictChain(hard, left, right),
					FromLeft:   left.LeftDepth,
					ToLeft:     right.LeftDepth,
					FromRight:  left.RightDepth,
					ToRight:    right.RightDepth,
				})
			}
		}
	}

	for i := 0; i+1 < len(hard); i++ {
		left := hard[i]
		right := hard[i+1]
		if math.Abs(right.LeftDepth-left.LeftDepth) <= epsilon || right.RightDepth < left.RightDepth {
			continue
		}
		stretch := (right.RightDepth - left.RightDepth) / (right.LeftDepth - left.LeftDepth)
		if stretch < settings.MinStretch-epsilon || stretch > settings.MaxStretch+epsilon {
			conflicts = append(conflicts, Conflict{
				Severity:   "blocking",
				Code:       "hard_stretch_violation",
				Message:    "相邻硬锦标之间的伸缩率超出可配置范围。",
				ControlIDs: []string{left.ID, right.ID},
				Chain:      []string{left.ID, right.ID},
				FromLeft:   left.LeftDepth,
				ToLeft:     right.LeftDepth,
				FromRight:  left.RightDepth,
				ToRight:    right.RightDepth,
				Stretch:    stretch,
			})
		}
	}

	for _, current := range soft {
		for _, fixed := range hard {
			if current.LeftDepth < fixed.LeftDepth && current.RightDepth > fixed.RightDepth+epsilon ||
				current.LeftDepth > fixed.LeftDepth && current.RightDepth < fixed.RightDepth-epsilon {
				conflicts = append(conflicts, Conflict{
					Severity:   "advisory",
					Code:       "soft_hard_crossed",
					Message:    "自动建议或软锦标与硬约束方向不一致，只能以偏离代价求解。",
					ControlIDs: []string{current.ID, fixed.ID},
				})
			}
		}
	}

	for _, proposal := range proposals {
		if proposal.Status != "proposed" {
			continue
		}
		for _, fixed := range controls {
			if math.Abs(proposal.LeftDepth-fixed.LeftDepth) <= epsilon && math.Abs(proposal.RightDepth-fixed.RightDepth) > epsilon {
				conflicts = append(conflicts, Conflict{
					Severity:    "advisory",
					Code:        "auto_manual_conflict",
					Message:     "自动建议与人工约束在同一左深给出不同右深。",
					ControlIDs:  []string{proposal.ID, fixed.ID},
					ProposalIDs: []string{proposal.ID},
					FromLeft:    proposal.LeftDepth,
					FromRight:   proposal.RightDepth,
					ToRight:     fixed.RightDepth,
				})
			}
		}
	}

	return conflicts
}

func shortestConflictChain(hard []Control, from, to Control) []string {
	type state struct {
		id     string
		parent string
	}
	queue := []string{from.ID}
	visited := map[string]state{from.ID: {id: from.ID}}
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		current := controlByID(hard, currentID)
		for _, next := range hard {
			if next.ID == current.ID {
				continue
			}
			if _, seen := visited[next.ID]; seen {
				continue
			}
			if next.LeftDepth > current.LeftDepth+epsilon && next.RightDepth < current.RightDepth-epsilon {
				visited[next.ID] = state{id: next.ID, parent: current.ID}
				if next.ID == to.ID {
					queue = nil
					break
				}
				queue = append(queue, next.ID)
			}
		}
	}

	end, ok := visited[to.ID]
	if !ok {
		return []string{from.ID, to.ID}
	}
	chain := []string{}
	for id := end.id; id != ""; {
		chain = append(chain, id)
		current := visited[id]
		if current.parent == "" {
			break
		}
		id = current.parent
	}
	for left, right := 0, len(chain)-1; left < right; left, right = left+1, right-1 {
		chain[left], chain[right] = chain[right], chain[left]
	}
	return chain
}

func controlByID(controls []Control, id string) Control {
	for _, control := range controls {
		if control.ID == id {
			return control
		}
	}
	return Control{ID: id}
}
