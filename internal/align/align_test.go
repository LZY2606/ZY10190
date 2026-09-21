package align

import (
	"fmt"
	"math"
	"testing"
)

func fixtureControls(t *testing.T) (RunData, []Control, []Control) {
	t.Helper()
	return Fixture()
}

func TestInitialFixtureReportsShortestCrossedHardChain(t *testing.T) {
	run, controls, proposals := fixtureControls(t)
	result := Solve(run, controls, proposals, DefaultSettings())
	if result.OK {
		t.Fatal("initial fixture must be infeasible while crossed hard controls are active")
	}
	var chain []string
	for _, conflict := range result.Conflicts {
		if conflict.Code == "hard_order_crossed" {
			chain = conflict.Chain
		}
	}
	if len(chain) != 2 || chain[0] != "x1" || chain[1] != "x2" {
		t.Fatalf("expected shortest chain [x1 x2], got %#v", chain)
	}
}

func TestRevokingEitherCrossedControlRestoresMonotonicSolution(t *testing.T) {
	run, controls, proposals := fixtureControls(t)
	for _, revoked := range []string{"x1", "x2"} {
		active := []Control{}
		for _, control := range controls {
			if control.ID != revoked {
				active = append(active, control)
			}
		}
		result := Solve(run, active, proposals, DefaultSettings())
		if !result.OK {
			t.Fatalf("revoking %s should restore feasibility: %s", revoked, result.Message)
		}
		if len(result.Candidates) != 3 {
			t.Fatalf("expected 3 retained candidate paths, got %d", len(result.Candidates))
		}
		if result.Candidates[1].DiffFromBestMax <= epsilon || result.Candidates[2].DiffFromBestMax <= epsilon {
			t.Fatalf("alternative candidates must retain geometric alignment differences: %#v", result.Candidates)
		}
		points := result.Candidates[0].Points
		if err := assertStrictMonotonic(points); err != nil {
			t.Fatalf("revoking %s: %v", revoked, err)
		}
		for _, hard := range active {
			if hard.Kind != "hard" {
				continue
			}
			if !containsPoint(points, hard.LeftDepth, hard.RightDepth) {
				t.Fatalf("hard control %s missing from path", hard.ID)
			}
		}
	}
}

func TestFixtureNeverFillsMissingSamplesWithZero(t *testing.T) {
	run, _, _ := fixtureControls(t)
	for _, curve := range run.Curves {
		for _, sample := range append(curve.SamplesA, curve.SamplesB...) {
			if sample.Value == 0 {
				t.Fatalf("curve %s contains zero-filled sample", curve.Key)
			}
		}
	}
}

func TestGapIsOnlyCrossedAsPairedNoDataBridge(t *testing.T) {
	run, controls, proposals := fixtureControls(t)
	active := []Control{}
	for _, control := range controls {
		if control.ID != "x2" {
			active = append(active, control)
		}
	}
	result := Solve(run, active, proposals, DefaultSettings())
	if !result.OK {
		t.Fatal(result.Message)
	}
	bridges := 0
	for _, segment := range result.Candidates[0].Segments {
		if segment.Kind != "gap_bridge" {
			continue
		}
		bridges++
		if math.Abs(segment.From.Left-1050) > epsilon || math.Abs(segment.To.Left-1054) > epsilon ||
			math.Abs(segment.From.Right-1085) > epsilon || math.Abs(segment.To.Right-1090) > epsilon {
			t.Fatalf("gap bridge cannot use nearby similar shape as shortcut: %#v", segment)
		}
		if math.Abs(segment.Similarity) > epsilon {
			t.Fatalf("similarity must not propagate across gap, got %f", segment.Similarity)
		}
	}
	if bridges != 1 {
		t.Fatalf("expected exactly one paired no-data bridge, got %d", bridges)
	}
}

func TestStretchViolationIsReported(t *testing.T) {
	run, controls, proposals := fixtureControls(t)
	controls = append(controls, Control{
		ID: "bad", Kind: "hard", Source: "manual", Status: "active",
		LeftDepth: 1085, RightDepth: 1145, CreatedSeq: 99,
	})
	settings := DefaultSettings()
	settings.MaxStretch = 1.5
	result := Solve(run, controls, proposals, settings)
	found := false
	for _, conflict := range result.Conflicts {
		if conflict.Code == "hard_stretch_violation" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected stretch violation in conflicts: %#v", result.Conflicts)
	}
}

func TestZeroDepthSegmentIsRejected(t *testing.T) {
	controls := []Control{
		{ID: "a", Kind: "hard", Status: "active", LeftDepth: 1010, RightDepth: 1042.5},
		{ID: "b", Kind: "hard", Status: "active", LeftDepth: 1010, RightDepth: 1050},
	}
	conflicts := AnalyzeConflicts(controls, nil, DefaultSettings())
	found := false
	for _, conflict := range conflicts {
		if conflict.Code == "hard_same_depth" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected same-depth hard conflict, got %#v", conflicts)
	}
}

func TestControlInsideNoDataIsRejected(t *testing.T) {
	run, controls, proposals := fixtureControls(t)
	controls = append(controls, Control{
		ID: "gap-control", Kind: "hard", Source: "manual", Status: "active",
		LeftDepth: 1052, RightDepth: 1087, CreatedSeq: 101,
	})
	result := Solve(run, controls, proposals, DefaultSettings())
	found := false
	for _, conflict := range result.Conflicts {
		if conflict.Code == "control_inside_no_data" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected no-data control conflict, got %#v", result.Conflicts)
	}
}

func assertStrictMonotonic(points []Point) error {
	for index := 1; index < len(points); index++ {
		if points[index].Left <= points[index-1].Left+epsilon {
			return fmt.Errorf("left depth not strictly increasing at %d", index)
		}
		if points[index].Right <= points[index-1].Right+epsilon {
			return fmt.Errorf("right depth not strictly increasing at %d", index)
		}
		slope := (points[index].Right - points[index-1].Right) / (points[index].Left - points[index-1].Left)
		if slope < 0.5-epsilon || slope > 2+epsilon {
			return fmt.Errorf("slope %f outside range at %d", slope, index)
		}
	}
	return nil
}

func containsPoint(points []Point, left, right float64) bool {
	for _, point := range points {
		if math.Abs(point.Left-left) <= epsilon && math.Abs(point.Right-right) <= epsilon {
			return true
		}
	}
	return false
}
