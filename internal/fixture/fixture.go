package fixture

import (
	"math"

	"depthalign/internal/domain"
)

const (
	AMin float64 = 10
	AMax float64 = 360
	BMin float64 = 10
	BMax float64 = 358
)

var (
	GapA = domain.Interval{From: 185.5, To: 189.5}
	GapB = domain.Interval{From: 184, To: 188}
)

func logicalDepthB(y float64) float64 {
	switch {
	case y <= 184:
		return 10 + (y-10)*1.01
	case y <= 188:
		return 185.5 + (y-184)*(GapA.To-GapA.From)/(GapB.To-GapB.From)
	default:
		return GapA.To + (y-GapB.To)*(AMax-GapA.To)/(BMax-GapB.To)
	}
}

func gamma(x float64) float64 {
	v := 74 + 16*math.Sin(x/12.5) + 7*math.Sin(x/4.7+0.8)
	v += gaussian(x, 90, 3.2, 31)
	v += gaussian(x, 214, 4.1, -24)
	v += gaussian(x, 285, 5.4, 20)
	return v
}

func resistivity(x float64) float64 {
	v := 18 + 5*math.Sin(x/17.3+1.4) + 2.2*math.Sin(x/5.1)
	v += gaussian(x, 116, 4.8, 17)
	v += gaussian(x, 232, 6.2, 13)
	v += gaussian(x, 310, 3.8, -9)
	return math.Max(2, v)
}

func gaussian(x, center, width, amplitude float64) float64 {
	d := x - center
	return amplitude * math.Exp(-(d*d)/(2*width*width))
}

func inGap(d float64, gap domain.Interval) bool {
	return d >= gap.From && d <= gap.To
}

func Build() domain.Dataset {
	markers := []domain.Marker{
		{ID: "h1", Label: "K1 硬地层锦标", PassAFrom: 30, PassBTo: 30.2, Kind: "hard", Origin: "fixture", Active: true, CreatedAt: 1},
		{ID: "h2", Label: "K2 硬地层锦标", PassAFrom: 120, PassBTo: 118.5, Kind: "hard", Origin: "fixture", Active: true, CreatedAt: 2},
		{ID: "h3", Label: "K3 硬地层锦标", PassAFrom: 322, PassBTo: 320, Kind: "hard", Origin: "fixture", Active: true, CreatedAt: 3},
		{ID: "x1", Label: "人工对应 X", PassAFrom: 55, PassBTo: 60, Kind: "hard", Origin: "manual", Active: true, CreatedAt: 4},
		{ID: "y1", Label: "人工对应 Y", PassAFrom: 65, PassBTo: 58.5, Kind: "hard", Origin: "manual", Active: true, CreatedAt: 5},
		{ID: "s60", Label: "自动建议：60 m 形状尖峰", PassAFrom: 60, PassBTo: 61, Kind: "soft", Origin: "auto", Active: false, CreatedAt: 6},
		{ID: "s90", Label: "自动建议：90 m 异常峰值", PassAFrom: 90, PassBTo: 86, Kind: "soft", Origin: "auto", Active: false, CreatedAt: 6},
	}

	var gammaA, gammaB, resA, resB []domain.Sample
	for x := AMin; x <= AMax+1e-9; x += 0.5 {
		if inGap(x, GapA) {
			continue
		}
		gammaA = append(gammaA, domain.Sample{Depth: round1(x), Value: round3(gamma(x))})
		resA = append(resA, domain.Sample{Depth: round1(x), Value: round3(resistivity(x))})
	}
	for y := BMin; y <= BMax+1e-9; y += 0.4 {
		yy := round1(y)
		if inGap(yy, GapB) {
			continue
		}
		x := logicalDepthB(yy)
		gammaB = append(gammaB, domain.Sample{Depth: yy, Value: round3(gamma(x))})
		resB = append(resB, domain.Sample{Depth: yy, Value: round3(resistivity(x))})
	}
	gammaB = append(gammaB, domain.Sample{Depth: BMax, Value: round3(gamma(logicalDepthB(BMax)))})
	resB = append(resB, domain.Sample{Depth: BMax, Value: round3(resistivity(logicalDepthB(BMax)))})

	curves := make(map[string]domain.Curve, 4)
	for name, pair := range map[string][2][]domain.Sample{
		"gamma":       {gammaA, gammaB},
		"resistivity": {resA, resB},
	} {
		curves[name+":A"] = domain.Curve{Name: name, Pass: "A", Samples: pair[0], Nodata: []domain.Interval{GapA}, MinDepth: AMin, MaxDepth: AMax}
		curves[name+":B"] = domain.Curve{Name: name, Pass: "B", Samples: pair[1], Nodata: []domain.Interval{GapB}, MinDepth: BMin, MaxDepth: BMax}
	}
	return domain.Dataset{Curves: curves, Markers: markers}
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
