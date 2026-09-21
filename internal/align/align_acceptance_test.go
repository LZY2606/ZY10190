package align_test

import (
	"math"
	"testing"

	"depthalign/internal/align"
	"depthalign/internal/domain"
	"depthalign/internal/fixture"
)

func solveFixtureWithout(ids ...string) domain.Result {
	data := fixture.Build()
	skip := map[string]bool{}
	for _, id := range ids {
		skip[id] = true
	}
	for i := range data.Markers {
		if skip[data.Markers[i].ID] {
			data.Markers[i].Active = false
		}
	}
	return align.Solve(data, align.DefaultConfig())
}

func TestInitialConflictIsShortestCrossingChain(t *testing.T) {
	result := solveFixtureWithout()
	if result.OK {
		t.Fatal("initial fixture must be blocked by crossed manual markers")
	}
	if len(result.Conflicts) != 1 {
		t.Fatalf("expected one shortest adjacent chain, got %d", len(result.Conflicts))
	}
	conflict := result.Conflicts[0]
	if conflict.Kind != "hard_crossing" || len(conflict.Chain) != 2 {
		t.Fatalf("unexpected conflict: %+v", conflict)
	}
	ids := []string{conflict.Chain[0].ID, conflict.Chain[1].ID}
	if ids[0] != "x1" || ids[1] != "y1" {
		t.Fatalf("shortest chain = %v, want x1->y1 regardless of creation order", ids)
	}
}

func TestInactiveAutoSuggestionStillReportsManualConflict(t *testing.T) {
	initial := solveFixtureWithout()
	if len(initial.Warnings) < 2 {
		t.Fatalf("expected auto suggestion conflicts with both manual markers, got %+v", initial.Warnings)
	}
	withoutX := solveFixtureWithout("x1")
	if !withoutX.OK {
		t.Fatalf("hard solve should be restored: %+v", withoutX.Conflicts)
	}
	if len(withoutX.Warnings) == 0 {
		t.Fatal("auto suggestion conflict with remaining manual marker should remain a warning")
	}
}

func TestDeletingEitherCrossedHardMarkerRestoresSolution(t *testing.T) {
	for _, removed := range []string{"x1", "y1"} {
		t.Run(removed, func(t *testing.T) {
			result := solveFixtureWithout(removed)
			if !result.OK || len(result.Candidates) != 3 {
				t.Fatalf("result ok=%v candidates=%d conflicts=%+v", result.OK, len(result.Candidates), result.Conflicts)
			}
			candidate := result.Candidates[0]
			assertPathShape(t, candidate.Points)
			assertHardMarkersPassed(t, candidate.Points, result, removed)
			if candidate.StretchMin < .75-1e-9 || candidate.StretchMax > 1.2+1e-9 {
				t.Fatalf("stretch range %.3f..%.3f outside bounds", candidate.StretchMin, candidate.StretchMax)
			}
			if len(candidate.NodataEdges) != 1 || candidate.NodataEdges[0].From != 185.5 || candidate.NodataEdges[0].To != 189.5 {
				t.Fatalf("gap not retained: %+v", candidate.NodataEdges)
			}
			if len(candidate.Violations) != 0 {
				t.Fatalf("bounded DP produced violations: %+v", candidate.Violations)
			}
			if len(candidate.Diffs) != 2 {
				t.Fatalf("expected gamma and resistivity diffs, got %d", len(candidate.Diffs))
			}
		})
	}
}

func TestGapSimilarityIsMaskedAndCannotShortcut(t *testing.T) {
	result := solveFixtureWithout("x1")
	candidate := result.Candidates[0]
	assertPathCrossesPhysicalGap(t, candidate.Points)
	for _, diff := range candidate.Diffs {
		var sum, count float64
		for _, sample := range diff.Data {
			if sample.From >= 185.5 && sample.From <= 189.5 {
				t.Fatalf("diff %s contains zero-filled gap sample at %.2f", diff.Name, sample.From)
			}
			sum += sample.Diff * sample.Diff
			count++
		}
		rms := math.Sqrt(sum / count)
		if math.Abs(rms-diff.RMS) > 1e-9 {
			t.Fatalf("reported RMS %.6f does not match non-gap samples %.6f", diff.RMS, rms)
		}
	}
}

func TestConfiguredStretchViolationsAreReportedAsSegments(t *testing.T) {
	data := fixture.Build()
	cfg := align.DefaultConfig()
	cfg.MaxStretch = 0.95
	result := align.Solve(data, cfg)
	if result.OK {
		t.Fatal("expected hard segment to be blocked under tightened bound")
	}
	found := false
	for _, conflict := range result.Conflicts {
		if conflict.Kind == "hard_stretch" && conflict.Segment != nil && conflict.Segment.Reason == "hard_stretch" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected hard_stretch segment report, got %+v", result.Conflicts)
	}
}

func TestSoftMarkersCanDeviateWithPenalty(t *testing.T) {
	result := solveFixtureWithout("x1", "y1")
	if !result.OK {
		t.Fatalf("expected solution with fixture hard markers: %+v", result.Conflicts)
	}
	before := result.Candidates[0].Costs.Soft
	data := fixture.Build()
	for i := range data.Markers {
		if data.Markers[i].ID == "x1" || data.Markers[i].ID == "y1" {
			data.Markers[i].Active = false
		}
		if data.Markers[i].ID == "s90" {
			data.Markers[i].Active = true
			data.Markers[i].PassBTo = 87
		}
	}
	withSoft := align.Solve(data, align.DefaultConfig())
	if !withSoft.OK {
		t.Fatalf("soft marker must not block solution: %+v", withSoft.Conflicts)
	}
	if withSoft.Candidates[0].Costs.Soft <= before+1e-9 {
		t.Fatalf("expected positive soft penalty, before=%v after=%v", before, withSoft.Candidates[0].Costs.Soft)
	}
}

func assertPathShape(t *testing.T, path []domain.Point) {
	t.Helper()
	for i := 1; i < len(path); i++ {
		if path[i].From <= path[i-1].From {
			t.Fatalf("mapping turned backwards in source at %d: %+v", i, path[i])
		}
		if path[i].To <= path[i-1].To {
			t.Fatalf("non-zero depth was compressed to a point at %.2f", path[i].From)
		}
	}
}

func assertHardMarkersPassed(t *testing.T, path []domain.Point, result domain.Result, removed string) {
	t.Helper()
	hard := map[float64]float64{}
	data := fixture.Build()
	for _, marker := range data.Markers {
		if marker.Active && marker.Kind == "hard" && marker.ID != removed {
			hard[marker.PassAFrom] = marker.PassBTo
		}
	}
	for x, wantY := range hard {
		y, ok := mappedAt(path, x)
		if !ok || math.Abs(y-wantY) > .051 {
			t.Fatalf("hard marker %.1f mapped to %.3f, want %.1f (ok=%v)", x, y, wantY, ok)
		}
	}
}

func assertPathCrossesPhysicalGap(t *testing.T, path []domain.Point) {
	t.Helper()
	before, beforeOK := mappedAt(path, 185.5)
	after, afterOK := mappedAt(path, 189.5)
	if !beforeOK || !afterOK || before > 184.05 || after < 187.95 || after <= before {
		t.Fatalf("path does not traverse physical B gap 184..188: %.3f..%.3f", before, after)
	}
}

func mappedAt(path []domain.Point, x float64) (float64, bool) {
	for i := 0; i+1 < len(path); i++ {
		if path[i].From <= x && x <= path[i+1].From {
			frac := (x - path[i].From) / (path[i+1].From - path[i].From)
			return path[i].To + frac*(path[i+1].To-path[i].To), true
		}
	}
	return 0, false
}

func TestCrossGapSimilarShapeCannotBeUsedAsShortcut(t *testing.T) {
	gapA := domain.Interval{From: 30, To: 34}
	gapB := domain.Interval{From: 28, To: 31}
	var samplesA, samplesB []domain.Sample
	for x := 10.0; x <= 50; x += 0.5 {
		if x >= gapA.From && x <= gapA.To {
			continue
		}
		samplesA = append(samplesA, domain.Sample{Depth: x, Value: math.Sin(x)})
	}
	for y := 10.0; y <= 48; y += 0.5 {
		if y >= gapB.From && y <= gapB.To {
			continue
		}
		samplesB = append(samplesB, domain.Sample{Depth: y, Value: math.Sin(y + 18)})
	}
	data := domain.Dataset{Curves: map[string]domain.Curve{
		"gamma:A": {Name: "gamma", Pass: "A", Samples: samplesA, Nodata: []domain.Interval{gapA}, MinDepth: 10, MaxDepth: 50},
		"gamma:B": {Name: "gamma", Pass: "B", Samples: samplesB, Nodata: []domain.Interval{gapB}, MinDepth: 10, MaxDepth: 48},
	}}
	result := align.Solve(data, align.DefaultConfig())
	if !result.OK {
		t.Fatalf("synthetic gap solve: %+v", result.Conflicts)
	}
	before, beforeOK := mappedAt(result.Candidates[0].Points, gapA.From)
	after, afterOK := mappedAt(result.Candidates[0].Points, gapA.To)
	if !beforeOK || !afterOK || before >= gapB.From || after <= gapB.To || after <= before {
		t.Fatalf("mapping skipped gap: %.2f..%.2f", before, after)
	}
}

func TestEmptyDatasetReturnsReadableConflict(t *testing.T) {
	result := align.Solve(domain.Dataset{}, align.DefaultConfig())
	if result.OK || len(result.Conflicts) != 1 || result.Conflicts[0].Kind != "no_curves" {
		t.Fatalf("unexpected empty dataset result: %+v", result.Conflicts)
	}
}
