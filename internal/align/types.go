package align

import "sort"

type Sample struct {
	Depth float64 `json:"depth"`
	Value float64 `json:"value"`
}

type Curve struct {
	Key      string   `json:"key"`
	Name     string   `json:"name"`
	Unit     string   `json:"unit"`
	Color    string   `json:"color"`
	SamplesA []Sample `json:"samplesA"`
	SamplesB []Sample `json:"samplesB"`
}

type Interval struct {
	From float64 `json:"from"`
	To   float64 `json:"to"`
}

type RunData struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	DepthUnit   string     `json:"depthUnit"`
	Curves      []Curve    `json:"curves"`
	GapsA       []Interval `json:"gapsA"`
	GapsB       []Interval `json:"gapsB"`
	DomainA     Interval   `json:"domainA"`
	DomainB     Interval   `json:"domainB"`
	Description string     `json:"description"`
}

type Control struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"`
	Source     string  `json:"source"`
	Status     string  `json:"status"`
	LeftDepth  float64 `json:"leftDepth"`
	RightDepth float64 `json:"rightDepth"`
	Penalty    float64 `json:"penalty,omitempty"`
	Note       string  `json:"note,omitempty"`
	CreatedSeq int64   `json:"createdSeq"`
}

type Settings struct {
	MinStretch        float64 `json:"minStretch"`
	MaxStretch        float64 `json:"maxStretch"`
	SoftWeight        float64 `json:"softWeight"`
	RegularityWeight  float64 `json:"regularityWeight"`
	MaxCandidates     int     `json:"maxCandidates"`
	SimilarityWindowM float64 `json:"similarityWindowM"`
}

func DefaultSettings() Settings {
	return Settings{
		MinStretch:        0.5,
		MaxStretch:        2.0,
		SoftWeight:        8.0,
		RegularityWeight:  0.02,
		MaxCandidates:     3,
		SimilarityWindowM: 8.0,
	}
}

type Costs struct {
	Similarity     float64 `json:"similarity"`
	SoftPenalty    float64 `json:"softPenalty"`
	Regularity     float64 `json:"regularity"`
	Total          float64 `json:"total"`
	GapBridgesM    float64 `json:"gapBridgesM"`
	EvaluatedEdges int     `json:"evaluatedEdges"`
}

type Point struct {
	Left     float64 `json:"left"`
	Right    float64 `json:"right"`
	Band     int     `json:"band"`
	Required bool    `json:"required,omitempty"`
}

type Segment struct {
	From        Point   `json:"from"`
	To          Point   `json:"to"`
	Stretch     float64 `json:"stretch"`
	Kind        string  `json:"kind"`
	Similarity  float64 `json:"similarity,omitempty"`
	Regularity  float64 `json:"regularity,omitempty"`
	SoftPenalty float64 `json:"softPenalty,omitempty"`
	Warning     string  `json:"warning,omitempty"`
}

type SoftDeviation struct {
	ControlID  string  `json:"controlId"`
	Expected   float64 `json:"expected"`
	Actual     float64 `json:"actual"`
	DeviationM float64 `json:"deviationM"`
	Cost       float64 `json:"cost"`
}

type Residual struct {
	CurveKey string  `json:"curveKey"`
	SSE      float64 `json:"sse"`
	SAE      float64 `json:"sae"`
	MaxAbs   float64 `json:"maxAbs"`
	Count    int     `json:"count"`
}

type Candidate struct {
	Rank            int             `json:"rank"`
	Points          []Point         `json:"points"`
	Segments        []Segment       `json:"segments"`
	Costs           Costs           `json:"costs"`
	SoftDeviations  []SoftDeviation `json:"softDeviations"`
	Residuals       []Residual      `json:"residuals"`
	GapBridgeCount  int             `json:"gapBridgeCount"`
	DiffFromBestMAD float64         `json:"diffFromBestMad"`
	DiffFromBestMax float64         `json:"diffFromBestMax"`
}

type Conflict struct {
	Severity    string   `json:"severity"`
	Code        string   `json:"code"`
	Message     string   `json:"message"`
	ControlIDs  []string `json:"controlIds"`
	Chain       []string `json:"chain,omitempty"`
	FromLeft    float64  `json:"fromLeft,omitempty"`
	ToLeft      float64  `json:"toLeft,omitempty"`
	FromRight   float64  `json:"fromRight,omitempty"`
	ToRight     float64  `json:"toRight,omitempty"`
	Stretch     float64  `json:"stretch,omitempty"`
	ProposalIDs []string `json:"proposalIds,omitempty"`
}

type SolveResult struct {
	OK                 bool        `json:"ok"`
	Settings           Settings    `json:"settings"`
	Candidates         []Candidate `json:"candidates"`
	Conflicts          []Conflict  `json:"conflicts"`
	NoDataSegments     []Segment   `json:"noDataSegments"`
	BoundaryViolations []Conflict  `json:"boundaryViolations"`
	Message            string      `json:"message"`
}

func ActiveControls(controls []Control) []Control {
	out := make([]Control, 0, len(controls))
	for _, c := range controls {
		if c.Status == "active" {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].LeftDepth == out[j].LeftDepth {
			return out[i].CreatedSeq < out[j].CreatedSeq
		}
		return out[i].LeftDepth < out[j].LeftDepth
	})
	return out
}
