package domain

type Sample struct {
	Depth float64 `json:"depth"`
	Value float64 `json:"value"`
}

type Interval struct {
	From float64 `json:"from"`
	To   float64 `json:"to"`
}

type Curve struct {
	Name     string     `json:"name"`
	Pass     string     `json:"pass"`
	Samples  []Sample   `json:"samples"`
	Nodata   []Interval `json:"nodata"`
	MinDepth float64    `json:"min_depth"`
	MaxDepth float64    `json:"max_depth"`
}

type Marker struct {
	ID        string  `json:"id"`
	Label     string  `json:"label"`
	PassAFrom float64 `json:"a_from"`
	PassBTo   float64 `json:"b_to"`
	Kind      string  `json:"kind"`
	Origin    string  `json:"origin"`
	Active    bool    `json:"active"`
	CreatedAt int64   `json:"created_at,omitempty"`
}

type CostBreakdown struct {
	Data    float64 `json:"data"`
	Stretch float64 `json:"stretch"`
	Soft    float64 `json:"soft"`
	Total   float64 `json:"total"`
}

type Point struct {
	From float64 `json:"from"`
	To   float64 `json:"to"`
}

type Candidate struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Points      []Point        `json:"points"`
	Costs       CostBreakdown  `json:"costs"`
	StretchMin  float64        `json:"stretch_min"`
	StretchMax  float64        `json:"stretch_max"`
	Violations  []SegmentIssue `json:"violations"`
	NodataEdges []Interval     `json:"nodata_edges"`
	Diffs       []CurveDiff    `json:"diffs"`
}

type DiffSample struct {
	From   float64 `json:"from"`
	To     float64 `json:"to"`
	AValue float64 `json:"a_value"`
	BValue float64 `json:"b_value"`
	Diff   float64 `json:"diff"`
}

type CurveDiff struct {
	Name string       `json:"name"`
	RMS  float64      `json:"rms"`
	Data []DiffSample `json:"data"`
}

type SegmentIssue struct {
	From       float64 `json:"from"`
	To         float64 `json:"to"`
	FromMapped float64 `json:"from_mapped"`
	ToMapped   float64 `json:"to_mapped"`
	Stretch    float64 `json:"stretch"`
	Reason     string  `json:"reason"`
}

type Conflict struct {
	Kind    string        `json:"kind"`
	Message string        `json:"message"`
	Chain   []Marker      `json:"chain"`
	Segment *SegmentIssue `json:"segment,omitempty"`
}

type Result struct {
	OK             bool        `json:"ok"`
	SelectedID     string      `json:"selected_id"`
	Candidates     []Candidate `json:"candidates"`
	Conflicts      []Conflict  `json:"conflicts"`
	Warnings       []Conflict  `json:"warnings"`
	NoDataSegments []Interval  `json:"no_data_segments"`
	Config         SolveConfig `json:"config"`
}

type SolveConfig struct {
	MinStretch float64 `json:"min_stretch"`
	MaxStretch float64 `json:"max_stretch"`
	SoftWeight float64 `json:"soft_weight"`
}

type Dataset struct {
	Curves  map[string]Curve `json:"curves"`
	Markers []Marker         `json:"markers"`
}
