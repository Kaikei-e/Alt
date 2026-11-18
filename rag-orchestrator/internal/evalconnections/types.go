package evalconnections

const (
	TargetChunkMeanNN        = 20
	TargetFirstChunkNN       = 12
	TargetChunkMeanRank5To15 = 10
	TargetRandom             = 6
	TotalSamplePairs         = TargetChunkMeanNN + TargetFirstChunkNN + TargetChunkMeanRank5To15 + TargetRandom

	ThresholdVersion  = 1
	MinConnectedShare = 0.60

	RepresentationSummaryVector = "summary_vector"
	StatusAvailable             = "available"
	StatusSkipped               = "skipped"
)

// CanonicalRepresentations lists all evaluated representations in display order.
var CanonicalRepresentations = []string{
	"first_chunk",
	"chunk_mean",
	"nearest_chunk_pair",
	RepresentationSummaryVector,
}

// Stratum names the sampling bucket a pair was drawn from.
type Stratum string

const (
	StratumChunkMeanNN        Stratum = "chunk_mean_nn"
	StratumFirstChunkNN       Stratum = "first_chunk_nn"
	StratumChunkMeanRank5To15 Stratum = "chunk_mean_rank_5_15"
	StratumRandom             Stratum = "random"
)

// PairKey is an order-independent key for an unordered pair of article IDs.
type PairKey struct {
	A string
	B string
}

// CanonicalPairKey returns a PairKey with A <= B so (a, b) and (b, a) match.
func CanonicalPairKey(a, b string) PairKey {
	if a <= b {
		return PairKey{A: a, B: b}
	}
	return PairKey{A: b, B: a}
}

// SelectedPair is one sampled pair before or after shuffling.
type SelectedPair struct {
	PairNumber int
	ArticleA   string
	ArticleB   string
	Stratum    Stratum
}

// SheetRow represents one row in the blind scoring CSV.
type SheetRow struct {
	PairNumber int
	TitleA     string
	URLA       string
	TitleB     string
	URLB       string
	Verdict    string
}

// KeyPair stores ground-truth stratum and similarities for a sampled pair.
type KeyPair struct {
	PairNumber   int                `json:"pair_number"`
	ArticleIDA   string             `json:"article_id_a"`
	ArticleIDB   string             `json:"article_id_b"`
	Stratum      Stratum            `json:"stratum"`
	Similarities map[string]float64 `json:"similarities"`
}

// KeyFile is the JSON sidecar written by sample and consumed by score.
type KeyFile struct {
	Seed             int64             `json:"seed"`
	ThresholdVersion int               `json:"threshold_version"`
	Representations  map[string]string `json:"representations"`
	Pairs            []KeyPair         `json:"pairs"`
}

// ThresholdEvaluation summarizes one candidate minimum-similarity threshold.
type ThresholdEvaluation struct {
	MinSimilarity     float64 `json:"min_similarity"`
	ProposedCount     int     `json:"proposed_count"`
	JudgedCount       int     `json:"judged_count"`
	ConnectedCount    int     `json:"connected_count"`
	NotConnectedCount int     `json:"not_connected_count"`
	UnsureCount       int     `json:"unsure_count"`
	ConnectedShare    float64 `json:"connected_share"`
	MeetsTarget       bool    `json:"meets_target"`
}

// RepresentationReport summarizes all candidate thresholds for one representation.
type RepresentationReport struct {
	Representation string                `json:"representation"`
	Status         string                `json:"status"`
	ScoredPairs    int                   `json:"scored_pairs"`
	Thresholds     []ThresholdEvaluation `json:"thresholds,omitempty"`
}

// Recommendation identifies the winning representation and threshold when Passed is true.
type Recommendation struct {
	Representation    string  `json:"representation"`
	MinSimilarity     float64 `json:"min_similarity"`
	ProposedCount     int     `json:"proposed_count"`
	JudgedCount       int     `json:"judged_count"`
	ConnectedCount    int     `json:"connected_count"`
	NotConnectedCount int     `json:"not_connected_count"`
	UnsureCount       int     `json:"unsure_count"`
	ConnectedShare    float64 `json:"connected_share"`
}

// ScoreReport is the complete evaluation report emitted by score.
type ScoreReport struct {
	Passed               bool                   `json:"passed"`
	ThresholdVersion     int                    `json:"threshold_version"`
	TargetConnectedShare float64                `json:"target_connected_share"`
	Recommendation       *Recommendation        `json:"recommendation,omitempty"`
	Message              string                 `json:"message"`
	Representations      []RepresentationReport `json:"representations"`
}
