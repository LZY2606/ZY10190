package align

import "math"

const epsilon = 1e-9

func Fixture() (RunData, []Control, []Control) {
	run := RunData{
		ID:          "run-ZY10190",
		Name:        "ZY10190 伽马-电阻率复测",
		DepthUnit:   "m",
		Description: "固定确定性 fixture：三个硬地层锦标、共同无数据区、初始顺序交叉的两条人工硬锦标。",
		GapsA:       []Interval{{From: 1051.0, To: 1053.0}},
		GapsB:       []Interval{{From: 1086.0, To: 1089.0}},
		DomainA:     Interval{From: 1000, To: 1100},
		DomainB:     Interval{From: 1030, To: 1147.5},
	}

	var grA, rtA, grB, rtB []Sample
	for depth := 1000.0; depth <= 1100.0+epsilon; depth += 2.0 {
		if inAnyInterval(depth, run.GapsA) {
			continue
		}
		grA = append(grA, Sample{Depth: depth, Value: gammaSignal(depth)})
		rtA = append(rtA, Sample{Depth: depth, Value: resistivitySignal(depth)})
	}
	for depth := 1030.0; depth <= 1147.5+epsilon; depth += 2.5 {
		if inAnyInterval(depth, run.GapsB) {
			continue
		}
		stratDepth := inverseRightMapping(depth)
		grB = append(grB, Sample{Depth: depth, Value: gammaSignal(stratDepth)})
		rtB = append(rtB, Sample{Depth: depth, Value: resistivitySignal(stratDepth)})
	}

	run.Curves = []Curve{
		{Key: "gr", Name: "自然伽马 GR", Unit: "API", Color: "#d97706", SamplesA: grA, SamplesB: grB},
		{Key: "rt", Name: "电阻率 RT", Unit: "Ω·m", Color: "#2563eb", SamplesA: rtA, SamplesB: rtB},
	}

	controls := []Control{
		{ID: "h1", Kind: "hard", Source: "formation", Status: "active", LeftDepth: 1010, RightDepth: 1042.5, Penalty: 0, Note: "砂岩层顶", CreatedSeq: 1},
		{ID: "h2", Kind: "hard", Source: "formation", Status: "active", LeftDepth: 1024, RightDepth: 1060, Penalty: 0, Note: "钙质夹层", CreatedSeq: 2},
		{ID: "h3", Kind: "hard", Source: "formation", Status: "active", LeftDepth: 1080, RightDepth: 1122.5, Penalty: 0, Note: "泥岩标志层", CreatedSeq: 3},
		{ID: "x1", Kind: "hard", Source: "manual", Status: "active", LeftDepth: 1030, RightDepth: 1071, Penalty: 0, Note: "人工对应 A", CreatedSeq: 4},
		{ID: "x2", Kind: "hard", Source: "manual", Status: "active", LeftDepth: 1038, RightDepth: 1070, Penalty: 0, Note: "人工对应 B", CreatedSeq: 5},
		{ID: "s1", Kind: "soft", Source: "manual", Status: "active", LeftDepth: 1020, RightDepth: 1057.5, Penalty: 0.002, Note: "软约束：弱权重，曲线证据可使其偏离", CreatedSeq: 6},
	}
	proposals := []Control{
		{ID: "p1", Kind: "soft", Source: "auto", Status: "proposed", LeftDepth: 1030, RightDepth: 1069, Penalty: 5, Note: "自动建议：GR 峰值；与人工 x1 同深不同右深", CreatedSeq: 7},
		{ID: "p2", Kind: "soft", Source: "auto", Status: "proposed", LeftDepth: 1070, RightDepth: 1110, Penalty: 5, Note: "自动建议：RT 阶跃", CreatedSeq: 8},
	}
	return run, controls, proposals
}

func RightMapping(left float64) float64 {
	switch {
	case left <= 1020:
		return 1030 + 1.25*(left-1000)
	case left <= 1050:
		return 1055 + (left - 1020)
	case left <= 1054:
		return 1085 + 1.25*(left-1050)
	default:
		return 1090 + 1.25*(left-1054)
	}
}

func inverseRightMapping(right float64) float64 {
	switch {
	case right <= 1055:
		return 1000 + (right-1030)/1.25
	case right <= 1085:
		return 1020 + (right - 1055)
	case right <= 1090:
		return 1050 + (right-1085)/1.25
	default:
		return 1054 + (right-1090)/1.25
	}
}

func gammaSignal(t float64) float64 {
	return 75 +
		12*math.Sin(0.22*t) +
		3.5*math.Sin(0.71*t+0.4) +
		8*gaussian(t, 1015, 1.8) -
		9*gaussian(t, 1038, 2.1) +
		7*gaussian(t, 1074, 2.4)
}

func resistivitySignal(t float64) float64 {
	return math.Max(4, 22+
		6*math.Sin(0.17*t+1.2)+
		2.4*math.Sin(0.53*t-0.7)-
		5*gaussian(t, 1015, 2.0)+
		7*gaussian(t, 1038, 2.2)-
		4*gaussian(t, 1074, 2.5))
}

func gaussian(x, center, width float64) float64 {
	d := (x - center) / width
	return math.Exp(-0.5 * d * d)
}

func inAnyInterval(depth float64, intervals []Interval) bool {
	for _, interval := range intervals {
		if depth > interval.From+epsilon && depth < interval.To-epsilon {
			return true
		}
	}
	return false
}
