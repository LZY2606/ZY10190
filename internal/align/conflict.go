package align

import (
	"fmt"
	"math"
	"sort"

	"depthalign/internal/domain"
)

func activeHard(markers []domain.Marker) []domain.Marker {
	var hard []domain.Marker
	for _, m := range markers {
		if m.Active && m.Kind == "hard" {
			hard = append(hard, m)
		}
	}
	sort.SliceStable(hard, func(i, j int) bool {
		if hard[i].PassAFrom == hard[j].PassAFrom {
			return hard[i].ID < hard[j].ID
		}
		return hard[i].PassAFrom < hard[j].PassAFrom
	})
	return hard
}

func crossingChains(markers []domain.Marker) []domain.Conflict {
	hard := activeHard(markers)
	var conflicts []domain.Conflict
	for i := 0; i+1 < len(hard); i++ {
		if hard[i].PassBTo >= hard[i+1].PassBTo {
			conflicts = append(conflicts, domain.Conflict{
				Kind:    "hard_crossing",
				Message: fmt.Sprintf("硬锦标顺序交叉：%s -> %s", hard[i].Label, hard[i+1].Label),
				Chain:   []domain.Marker{hard[i], hard[i+1]},
			})
		}
	}
	return conflicts
}

func stretchConflicts(markers []domain.Marker, cfg domain.SolveConfig) []domain.Conflict {
	hard := activeHard(markers)
	var conflicts []domain.Conflict
	for i := 0; i+1 < len(hard); i++ {
		if hard[i].PassBTo >= hard[i+1].PassBTo {
			continue
		}
		dx := hard[i+1].PassAFrom - hard[i].PassAFrom
		dy := hard[i+1].PassBTo - hard[i].PassBTo
		if dx <= 0 || dy <= 0 {
			continue
		}
		stretch := dy / dx
		if stretch < cfg.MinStretch-1e-9 || stretch > cfg.MaxStretch+1e-9 {
			issue := domain.SegmentIssue{
				From:       hard[i].PassAFrom,
				To:         hard[i+1].PassAFrom,
				FromMapped: hard[i].PassBTo,
				ToMapped:   hard[i+1].PassBTo,
				Stretch:    stretch,
				Reason:     "hard_stretch",
			}
			conflicts = append(conflicts, domain.Conflict{
				Kind:    "hard_stretch",
				Message: fmt.Sprintf("硬锦标段伸缩率 %.3f 超出 [%.3f, %.3f]", stretch, cfg.MinStretch, cfg.MaxStretch),
				Chain:   []domain.Marker{hard[i], hard[i+1]},
				Segment: &issue,
			})
		}
	}
	return conflicts
}

func markerWarnings(markers []domain.Marker, cfg domain.SolveConfig) []domain.Conflict {
	var warnings []domain.Conflict
	hard := activeHard(markers)
	for _, m := range markers {
		if m.Kind != "soft" || (!m.Active && m.Origin != "auto") {
			continue
		}
		left, right := neighbors(hard, m.PassAFrom)
		if left != nil && left.PassBTo > m.PassBTo {
			warnings = append(warnings, conflictWith("soft_crossing", m, *left))
		}
		if right != nil && right.PassBTo < m.PassBTo {
			warnings = append(warnings, conflictWith("soft_crossing", m, *right))
		}
		if left != nil {
			checkSoftStretch(&warnings, *left, m, cfg)
		}
		if right != nil {
			checkSoftStretch(&warnings, m, *right, cfg)
		}
	}
	return warnings
}

func neighbors(hard []domain.Marker, x float64) (*domain.Marker, *domain.Marker) {
	i := sort.Search(len(hard), func(i int) bool { return hard[i].PassAFrom >= x })
	var left, right *domain.Marker
	if i < len(hard) && hard[i].PassAFrom == x {
		return &hard[i], &hard[i]
	}
	if i > 0 {
		left = &hard[i-1]
	}
	if i < len(hard) {
		right = &hard[i]
	}
	return left, right
}

func checkSoftStretch(out *[]domain.Conflict, left, right domain.Marker, cfg domain.SolveConfig) {
	dx := right.PassAFrom - left.PassAFrom
	dy := right.PassBTo - left.PassBTo
	if dx <= 0 || dy <= 0 {
		return
	}
	stretch := dy / dx
	if stretch < cfg.MinStretch-1e-9 || stretch > cfg.MaxStretch+1e-9 {
		*out = append(*out, conflictWith("soft_stretch", right, left))
	}
}

func conflictWith(kind string, a, b domain.Marker) domain.Conflict {
	return domain.Conflict{
		Kind:    kind,
		Message: fmt.Sprintf("自动/软锦标 %s 与人工约束 %s 冲突", a.Label, b.Label),
		Chain:   []domain.Marker{a, b},
	}
}

func gridStretchConflicts(markers []domain.Marker, x0, y0, x1, y1 float64, nx, ny int, cfg domain.SolveConfig) []domain.Conflict {
	hard := activeHard(markers)
	if len(hard) == 0 {
		return nil
	}
	anchors := []domain.Marker{{PassAFrom: x0, PassBTo: y0, Label: "A/B 井段起点"}}
	anchors = append(anchors, hard...)
	anchors = append(anchors, domain.Marker{PassAFrom: x1, PassBTo: y1, Label: "A/B 井段终点"})
	minD := int(math.Ceil(cfg.MinStretch*SourceStep/TargetStep - 1e-9))
	maxD := int(math.Floor(cfg.MaxStretch*SourceStep/TargetStep + 1e-9))
	if minD < 1 || maxD < minD {
		return []domain.Conflict{{Kind: "stretch_grid", Message: "伸缩率配置在当前求解网格上没有可用步长"}}
	}
	var conflicts []domain.Conflict
	for i := 0; i+1 < len(anchors); i++ {
		left, right := anchors[i], anchors[i+1]
		layers := int(math.Round((right.PassAFrom - left.PassAFrom) / SourceStep))
		dy := int(math.Round((right.PassBTo - left.PassBTo) / TargetStep))
		if layers <= 0 || dy < minD*layers || dy > maxD*layers {
			issue := domain.SegmentIssue{
				From:       left.PassAFrom,
				To:         right.PassAFrom,
				FromMapped: left.PassBTo,
				ToMapped:   right.PassBTo,
				Stretch:    safeSlope(left, right),
				Reason:     "grid_stretch",
			}
			conflicts = append(conflicts, domain.Conflict{
				Kind:    "hard_stretch",
				Message: fmt.Sprintf("锦标段伸缩率 %.3f 在 %.2f/%.2f m 网格上不可达", issue.Stretch, SourceStep, TargetStep),
				Chain:   chainWithoutSynthetic(left, right),
				Segment: &issue,
			})
		}
	}
	return conflicts
}

func safeSlope(left, right domain.Marker) float64 {
	dx := right.PassAFrom - left.PassAFrom
	if dx == 0 {
		return 0
	}
	return (right.PassBTo - left.PassBTo) / dx
}

func chainWithoutSynthetic(left, right domain.Marker) []domain.Marker {
	var chain []domain.Marker
	if left.ID != "" {
		chain = append(chain, left)
	}
	if right.ID != "" {
		chain = append(chain, right)
	}
	return chain
}

func datasetConflicts(data domain.Dataset, bounds sourceTargetBounds) []domain.Conflict {
	if len(data.Curves) == 0 {
		return []domain.Conflict{{Kind: "no_curves", Message: "没有可对齐的测井曲线"}}
	}
	for _, marker := range data.Markers {
		if !marker.Active || (marker.Kind != "hard" && marker.Kind != "soft") {
			continue
		}
		if math.IsNaN(marker.PassAFrom) || math.IsNaN(marker.PassBTo) ||
			marker.PassAFrom < bounds.aMin || marker.PassAFrom > bounds.aMax ||
			marker.PassBTo < bounds.bMin || marker.PassBTo > bounds.bMax {
			return []domain.Conflict{{
				Kind:    "marker_out_of_range",
				Message: fmt.Sprintf("锦标 %s 超出曲线深度范围", marker.Label),
				Chain:   []domain.Marker{marker},
			}}
		}
	}
	return nil
}

type sourceTargetBounds struct {
	aMin, aMax, bMin, bMax float64
}
