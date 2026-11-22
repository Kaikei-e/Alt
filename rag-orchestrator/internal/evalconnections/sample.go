package evalconnections

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/fsperm"
	"rag-orchestrator/internal/port"
	"rag-orchestrator/internal/usecase"
)

const (
	rank5To15MinIdx    = 4  // 1-based rank 5
	rank5To15MaxIdx    = 14 // 1-based rank 15
	summaryEmbedBatch  = 32
	sheetFileName      = "scoring_sheet.csv"
	keyFileName        = "key.json"
	defaultSeedMixMask = uint64(0x9e3779b97f4a7c15)
)

var sheetCSVHeader = []string{"pair_number", "title_a", "url_a", "title_b", "url_b", "verdict"}

// ParseReadIDs reads one article ID per line, trimming whitespace, ignoring blank
// lines and '#' comments, and deduplicating while preserving first-seen order.
func ParseReadIDs(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	seen := make(map[string]struct{})
	var ids []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, dup := seen[line]; dup {
			continue
		}
		seen[line] = struct{}{}
		ids = append(ids, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read article ids: %w", err)
	}
	return ids, nil
}

// ParseTitlesTSV parses an optional TSV file of `article_id<TAB>title[<TAB>url]`.
func ParseTitlesTSV(r io.Reader) (map[string]domain.RagDocumentMetadata, error) {
	scanner := bufio.NewScanner(r)
	out := make(map[string]domain.RagDocumentMetadata)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimRight(scanner.Text(), "\r\n")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			return nil, fmt.Errorf("titles TSV line %d: expected at least 2 tab-separated columns", lineNum)
		}
		id := strings.TrimSpace(parts[0])
		if id == "" {
			return nil, fmt.Errorf("titles TSV line %d: empty article_id", lineNum)
		}
		meta := domain.RagDocumentMetadata{
			ArticleID: id,
			Title:     strings.TrimSpace(parts[1]),
		}
		if len(parts) >= 3 {
			meta.URL = strings.TrimSpace(parts[2])
		}
		out[id] = meta
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read titles TSV: %w", err)
	}
	return out, nil
}

// ParseSummariesTSV parses an optional TSV file of `article_id<TAB>summary_text`.
func ParseSummariesTSV(r io.Reader) (map[string]string, error) {
	scanner := bufio.NewScanner(r)
	// Allow long summary texts (up to 1 MiB per line).
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	out := make(map[string]string)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimRight(scanner.Text(), "\r\n")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 2 {
			return nil, fmt.Errorf("summaries TSV line %d: expected 2 tab-separated columns (article_id<TAB>summary)", lineNum)
		}
		id := strings.TrimSpace(parts[0])
		summary := strings.TrimSpace(parts[1])
		if id == "" {
			return nil, fmt.Errorf("summaries TSV line %d: empty article_id", lineNum)
		}
		if summary != "" {
			out[id] = summary
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read summaries TSV: %w", err)
	}
	return out, nil
}

func newSeededRNG(seed int64) *rand.Rand {
	// #nosec G115 -- only the bit pattern seeds the generator
	u := uint64(seed)
	// #nosec G404 -- a reproducible sample needs a seeded generator, not a secret
	return rand.New(rand.NewPCG(u, u^defaultSeedMixMask))
}

// SelectSamplePairs deterministically selects 48 unique unordered article pairs:
// 20 chunk_mean nearest neighbours, 12 first_chunk nearest neighbours,
// 10 pairs from chunk_mean ranks 5-15, and 6 uniformly random pairs, then shuffles them.
func SelectSamplePairs(
	indexedIDs []string,
	chunkMeanNeighbors map[string][]domain.CandidateSimilarityItem,
	firstChunkNeighbors map[string][]domain.CandidateSimilarityItem,
	seed int64,
) ([]SelectedPair, error) {
	sortedIDs := append([]string(nil), indexedIDs...)
	sort.Strings(sortedIDs)

	n := len(sortedIDs)
	if n*(n-1)/2 < TotalSamplePairs {
		return nil, fmt.Errorf("need at least %d distinct pairs among indexed read articles (got %d articles = %d pairs)", TotalSamplePairs, n, n*(n-1)/2)
	}

	rng := newSeededRNG(seed)
	used := make(map[PairKey]struct{}, TotalSamplePairs)
	selected := make([]SelectedPair, 0, TotalSamplePairs)

	addNearest := func(neighbors map[string][]domain.CandidateSimilarityItem, target int, stratum Stratum) error {
		added := 0
		maxRank := 0
		for _, id := range sortedIDs {
			if l := len(neighbors[id]); l > maxRank {
				maxRank = l
			}
		}
		for rank := 0; rank < maxRank && added < target; rank++ {
			perm := rng.Perm(len(sortedIDs))
			for _, idx := range perm {
				if added >= target {
					break
				}
				seedID := sortedIDs[idx]
				nbs := neighbors[seedID]
				if rank >= len(nbs) {
					continue
				}
				nbID := nbs[rank].ArticleID
				if nbID == "" || nbID == seedID {
					continue
				}
				key := CanonicalPairKey(seedID, nbID)
				if _, exists := used[key]; exists {
					continue
				}
				used[key] = struct{}{}
				selected = append(selected, SelectedPair{
					ArticleA: seedID,
					ArticleB: nbID,
					Stratum:  stratum,
				})
				added++
			}
		}
		if added < target {
			return fmt.Errorf("not enough unique pairs for stratum %s: got %d, want %d", stratum, added, target)
		}
		return nil
	}

	if err := addNearest(chunkMeanNeighbors, TargetChunkMeanNN, StratumChunkMeanNN); err != nil {
		return nil, err
	}
	if err := addNearest(firstChunkNeighbors, TargetFirstChunkNN, StratumFirstChunkNN); err != nil {
		return nil, err
	}

	// Stratum 3: 10 pairs from chunk_mean ranks 5-15 (0-based indices 4..14).
	rankPoolSeen := make(map[PairKey]struct{})
	var rankPool []SelectedPair
	for _, seedID := range sortedIDs {
		nbs := chunkMeanNeighbors[seedID]
		for r := rank5To15MinIdx; r <= rank5To15MaxIdx && r < len(nbs); r++ {
			nbID := nbs[r].ArticleID
			if nbID == "" || nbID == seedID {
				continue
			}
			key := CanonicalPairKey(seedID, nbID)
			if _, exists := used[key]; exists {
				continue
			}
			if _, seen := rankPoolSeen[key]; seen {
				continue
			}
			rankPoolSeen[key] = struct{}{}
			rankPool = append(rankPool, SelectedPair{
				ArticleA: seedID,
				ArticleB: nbID,
				Stratum:  StratumChunkMeanRank5To15,
			})
		}
	}
	if len(rankPool) < TargetChunkMeanRank5To15 {
		return nil, fmt.Errorf("not enough unused pairs in chunk_mean ranks 5-15: got %d, want %d", len(rankPool), TargetChunkMeanRank5To15)
	}
	rng.Shuffle(len(rankPool), func(i, j int) {
		rankPool[i], rankPool[j] = rankPool[j], rankPool[i]
	})
	for i := 0; i < TargetChunkMeanRank5To15; i++ {
		p := rankPool[i]
		used[CanonicalPairKey(p.ArticleA, p.ArticleB)] = struct{}{}
		selected = append(selected, p)
	}

	// Stratum 4: 6 uniformly random pairs not already in used.
	randomAdded := 0
	maxAttempts := TargetRandom * 200
	for attempts := 0; attempts < maxAttempts && randomAdded < TargetRandom; attempts++ {
		i := rng.IntN(n)
		j := rng.IntN(n - 1)
		if j >= i {
			j++
		}
		key := CanonicalPairKey(sortedIDs[i], sortedIDs[j])
		if _, exists := used[key]; exists {
			continue
		}
		used[key] = struct{}{}
		selected = append(selected, SelectedPair{
			ArticleA: sortedIDs[i],
			ArticleB: sortedIDs[j],
			Stratum:  StratumRandom,
		})
		randomAdded++
	}
	if randomAdded < TargetRandom {
		var remaining []PairKey
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				key := CanonicalPairKey(sortedIDs[i], sortedIDs[j])
				if _, exists := used[key]; !exists {
					remaining = append(remaining, key)
				}
			}
		}
		rng.Shuffle(len(remaining), func(i, j int) {
			remaining[i], remaining[j] = remaining[j], remaining[i]
		})
		for _, key := range remaining {
			if randomAdded >= TargetRandom {
				break
			}
			used[key] = struct{}{}
			selected = append(selected, SelectedPair{
				ArticleA: key.A,
				ArticleB: key.B,
				Stratum:  StratumRandom,
			})
			randomAdded++
		}
	}
	if randomAdded < TargetRandom {
		return nil, fmt.Errorf("not enough unused pairs for random stratum: got %d, want %d", randomAdded, TargetRandom)
	}

	// Randomly orient (ArticleA, ArticleB) and shuffle the 48 pairs so neither
	// row order nor column placement leaks stratum information.
	for i := range selected {
		if rng.IntN(2) == 1 {
			selected[i].ArticleA, selected[i].ArticleB = selected[i].ArticleB, selected[i].ArticleA
		}
	}
	rng.Shuffle(len(selected), func(i, j int) {
		selected[i], selected[j] = selected[j], selected[i]
	})
	for i := range selected {
		selected[i].PairNumber = i + 1
	}

	return selected, nil
}

// CosineSimilarity computes cosine similarity between two equal-length non-zero vectors.
func CosineSimilarity(a, b []float32) (float64, error) {
	if len(a) == 0 || len(a) != len(b) {
		return 0, fmt.Errorf("vector length mismatch: %d vs %d", len(a), len(b))
	}
	var dot, normA, normB float64
	for i := range a {
		fa := float64(a[i])
		fb := float64(b[i])
		dot += fa * fb
		normA += fa * fa
		normB += fb * fb
	}
	if normA == 0 || normB == 0 {
		return 0, fmt.Errorf("cannot compute cosine similarity for zero vector")
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB)), nil
}

// WriteScoringSheetCSV writes the blind scoring CSV with a blank verdict column.
func WriteScoringSheetCSV(w io.Writer, rows []SheetRow) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(sheetCSVHeader); err != nil {
		return fmt.Errorf("write CSV header: %w", err)
	}
	for _, r := range rows {
		if err := cw.Write([]string{
			strconv.Itoa(r.PairNumber),
			r.TitleA,
			r.URLA,
			r.TitleB,
			r.URLB,
			"",
		}); err != nil {
			return fmt.Errorf("write CSV row %d: %w", r.PairNumber, err)
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("flush CSV: %w", err)
	}
	return nil
}

// ReadScoringSheetCSV reads and validates a scoring sheet CSV.
func ReadScoringSheetCSV(r io.Reader) ([]SheetRow, error) {
	cr := csv.NewReader(r)
	records, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse CSV: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("scoring sheet CSV is empty")
	}
	header := records[0]
	if len(header) != len(sheetCSVHeader) {
		return nil, fmt.Errorf("unexpected CSV column count: got %d, want %d", len(header), len(sheetCSVHeader))
	}
	for i, want := range sheetCSVHeader {
		if strings.TrimSpace(header[i]) != want {
			return nil, fmt.Errorf("unexpected CSV column %d: got %q, want %q", i, header[i], want)
		}
	}

	rows := make([]SheetRow, 0, len(records)-1)
	for i, rec := range records[1:] {
		num, err := strconv.Atoi(strings.TrimSpace(rec[0]))
		if err != nil || num <= 0 {
			return nil, fmt.Errorf("row %d: invalid pair_number %q", i+2, rec[0])
		}
		rows = append(rows, SheetRow{
			PairNumber: num,
			TitleA:     rec[1],
			URLA:       rec[2],
			TitleB:     rec[3],
			URLB:       rec[4],
			Verdict:    strings.ToLower(strings.TrimSpace(rec[5])),
		})
	}
	return rows, nil
}

// WriteKeyJSON writes the JSON key file.
func WriteKeyJSON(w io.Writer, key KeyFile) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(key); err != nil {
		return fmt.Errorf("encode key JSON: %w", err)
	}
	return nil
}

// ReadKeyJSON parses a key JSON file.
func ReadKeyJSON(r io.Reader) (KeyFile, error) {
	var key KeyFile
	if err := json.NewDecoder(r).Decode(&key); err != nil {
		return KeyFile{}, fmt.Errorf("decode key JSON: %w", err)
	}
	return key, nil
}

// SampleOptions configures a SampleRunner execution.
type SampleOptions struct {
	UserID         string
	ReadIDs        []string
	OutDir         string
	Seed           int64
	TitleOverrides map[string]domain.RagDocumentMetadata
	Summaries      map[string]string
}

// SampleOutput reports the written artifact paths and counts.
type SampleOutput struct {
	SheetPath           string
	KeyPath             string
	PairCount           int
	IndexedReadArticles int
	SummaryVectorStatus string
}

// SampleRunner builds the 48-pair blind evaluation sheet and key file using
// CandidateSimilarityUsecase and ArticleMetadataReader.
type SampleRunner struct {
	simUC      usecase.CandidateSimilarityUsecase
	metaReader port.ArticleMetadataReader
	encoder    domain.VectorEncoder
}

// NewSampleRunner creates a SampleRunner. encoder may be nil when summaries are not supplied.
func NewSampleRunner(
	simUC usecase.CandidateSimilarityUsecase,
	metaReader port.ArticleMetadataReader,
	encoder domain.VectorEncoder,
) *SampleRunner {
	return &SampleRunner{
		simUC:      simUC,
		metaReader: metaReader,
		encoder:    encoder,
	}
}

// Run executes pair selection, representation scoring, and writes scoring_sheet.csv and key.json.
func (r *SampleRunner) Run(ctx context.Context, opts SampleOptions) (*SampleOutput, error) {
	if len(opts.ReadIDs) < 2 {
		return nil, fmt.Errorf("need at least 2 read article IDs")
	}

	chunkMeanNeighbors := make(map[string][]domain.CandidateSimilarityItem, len(opts.ReadIDs))
	firstChunkNeighbors := make(map[string][]domain.CandidateSimilarityItem, len(opts.ReadIDs))
	var indexedIDs []string

	for _, seedID := range opts.ReadIDs {
		items, seedIndexed, err := r.queryChunked(ctx, opts.UserID, seedID, opts.ReadIDs, domain.SimilarityRepChunkMean)
		if err != nil {
			return nil, fmt.Errorf("compute chunk_mean for seed %s: %w", seedID, err)
		}
		if !seedIndexed {
			continue
		}
		indexedIDs = append(indexedIDs, seedID)
		chunkMeanNeighbors[seedID] = items
	}

	for _, seedID := range indexedIDs {
		items, seedIndexed, err := r.queryChunked(ctx, opts.UserID, seedID, indexedIDs, domain.SimilarityRepFirstChunk)
		if err != nil {
			return nil, fmt.Errorf("compute first_chunk for seed %s: %w", seedID, err)
		}
		if seedIndexed {
			firstChunkNeighbors[seedID] = items
		}
	}

	pairs, err := SelectSamplePairs(indexedIDs, chunkMeanNeighbors, firstChunkNeighbors, opts.Seed)
	if err != nil {
		return nil, err
	}

	// Build lookup maps for chunk_mean and first_chunk similarities.
	cmLookup := toSimilarityLookup(chunkMeanNeighbors)
	fcLookup := toSimilarityLookup(firstChunkNeighbors)

	// Query nearest_chunk_pair only for the 48 selected pairs, grouped by ArticleA.
	npLookup, err := r.computeNearestChunkPairs(ctx, opts.UserID, pairs)
	if err != nil {
		return nil, err
	}

	uniqueIDs := collectUniquePairArticleIDs(pairs)
	dbMeta, err := r.metaReader.GetArticleMetadataByIDs(ctx, uniqueIDs, parseUUIDOrPanic(opts.UserID))
	if err != nil {
		return nil, fmt.Errorf("load article metadata: %w", err)
	}

	summaryStatus := StatusSkipped
	var summarySims map[PairKey]float64
	if len(opts.Summaries) > 0 && r.encoder != nil {
		summarySims, err = r.computeSummarySimilarities(ctx, uniqueIDs, pairs, opts.Summaries)
		if err != nil {
			return nil, err
		}
		summaryStatus = StatusAvailable
	}

	repStatus := map[string]string{
		"first_chunk":               StatusAvailable,
		"chunk_mean":                StatusAvailable,
		"nearest_chunk_pair":        StatusAvailable,
		RepresentationSummaryVector: summaryStatus,
	}

	rows := make([]SheetRow, 0, len(pairs))
	keyPairs := make([]KeyPair, 0, len(pairs))
	for _, p := range pairs {
		metaA := resolveMetadata(p.ArticleA, dbMeta, opts.TitleOverrides)
		metaB := resolveMetadata(p.ArticleB, dbMeta, opts.TitleOverrides)

		rows = append(rows, SheetRow{
			PairNumber: p.PairNumber,
			TitleA:     metaA.Title,
			URLA:       metaA.URL,
			TitleB:     metaB.Title,
			URLB:       metaB.URL,
		})

		sims := make(map[string]float64, 4)
		if sim, ok := lookupUndirected(cmLookup, p.ArticleA, p.ArticleB); ok {
			sims["chunk_mean"] = sim
		}
		if sim, ok := lookupUndirected(fcLookup, p.ArticleA, p.ArticleB); ok {
			sims["first_chunk"] = sim
		}
		if sim, ok := npLookup[CanonicalPairKey(p.ArticleA, p.ArticleB)]; ok {
			sims["nearest_chunk_pair"] = sim
		}
		if summaryStatus == StatusAvailable {
			if sim, ok := summarySims[CanonicalPairKey(p.ArticleA, p.ArticleB)]; ok {
				sims[RepresentationSummaryVector] = sim
			}
		}

		keyPairs = append(keyPairs, KeyPair{
			PairNumber:   p.PairNumber,
			ArticleIDA:   p.ArticleA,
			ArticleIDB:   p.ArticleB,
			Stratum:      p.Stratum,
			Similarities: sims,
		})
	}

	if err := os.MkdirAll(opts.OutDir, fsperm.PrivateDir); err != nil {
		return nil, fmt.Errorf("create output directory %s: %w", opts.OutDir, err)
	}

	sheetPath := filepath.Join(opts.OutDir, sheetFileName)
	sheetFile, err := os.Create(sheetPath) // #nosec G304 -- operator CLI output path
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", sheetPath, err)
	}
	if err := WriteScoringSheetCSV(sheetFile, rows); err != nil {
		_ = sheetFile.Close()
		return nil, err
	}
	if err := sheetFile.Close(); err != nil {
		return nil, fmt.Errorf("close %s: %w", sheetPath, err)
	}

	keyPath := filepath.Join(opts.OutDir, keyFileName)
	keyFile, err := os.Create(keyPath) // #nosec G304 -- operator CLI output path
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", keyPath, err)
	}
	key := KeyFile{
		Seed:             opts.Seed,
		ThresholdVersion: ThresholdVersion,
		Representations:  repStatus,
		Pairs:            keyPairs,
	}
	if err := WriteKeyJSON(keyFile, key); err != nil {
		_ = keyFile.Close()
		return nil, err
	}
	if err := keyFile.Close(); err != nil {
		return nil, fmt.Errorf("close %s: %w", keyPath, err)
	}

	return &SampleOutput{
		SheetPath:           sheetPath,
		KeyPath:             keyPath,
		PairCount:           len(pairs),
		IndexedReadArticles: len(indexedIDs),
		SummaryVectorStatus: summaryStatus,
	}, nil
}

func (r *SampleRunner) queryChunked(
	ctx context.Context,
	userID string,
	seedID string,
	allCandidates []string,
	rep domain.SimilarityRepresentation,
) ([]domain.CandidateSimilarityItem, bool, error) {
	maxBatch := domain.MaxCandidateSimilarityArticles
	if len(allCandidates) <= maxBatch {
		out, err := r.simUC.Execute(ctx, usecase.CandidateSimilarityInput{
			UserID:              userID,
			SeedArticleID:       seedID,
			CandidateArticleIDs: allCandidates,
			Representation:      string(rep),
		})
		if err != nil {
			return nil, false, err
		}
		return out.Results, out.SeedIndexed, nil
	}

	var merged []domain.CandidateSimilarityItem
	seedIndexed := false
	for start := 0; start < len(allCandidates); start += maxBatch {
		end := start + maxBatch
		if end > len(allCandidates) {
			end = len(allCandidates)
		}
		out, err := r.simUC.Execute(ctx, usecase.CandidateSimilarityInput{
			UserID:              userID,
			SeedArticleID:       seedID,
			CandidateArticleIDs: allCandidates[start:end],
			Representation:      string(rep),
		})
		if err != nil {
			return nil, false, err
		}
		if !out.SeedIndexed {
			return nil, false, nil
		}
		seedIndexed = true
		merged = append(merged, out.Results...)
	}

	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Similarity != merged[j].Similarity {
			return merged[i].Similarity > merged[j].Similarity
		}
		return merged[i].ArticleID < merged[j].ArticleID
	})
	return merged, seedIndexed, nil
}

func (r *SampleRunner) computeNearestChunkPairs(ctx context.Context, userID string, pairs []SelectedPair) (map[PairKey]float64, error) {
	bySeed := make(map[string][]string)
	for _, p := range pairs {
		bySeed[p.ArticleA] = append(bySeed[p.ArticleA], p.ArticleB)
	}
	seeds := make([]string, 0, len(bySeed))
	for seed := range bySeed {
		seeds = append(seeds, seed)
	}
	sort.Strings(seeds)

	outMap := make(map[PairKey]float64, len(pairs))
	for _, seed := range seeds {
		res, err := r.simUC.Execute(ctx, usecase.CandidateSimilarityInput{
			UserID:              userID,
			SeedArticleID:       seed,
			CandidateArticleIDs: bySeed[seed],
			Representation:      string(domain.SimilarityRepNearestChunkPair),
		})
		if err != nil {
			return nil, fmt.Errorf("compute nearest_chunk_pair for %s: %w", seed, err)
		}
		for _, item := range res.Results {
			outMap[CanonicalPairKey(seed, item.ArticleID)] = item.Similarity
		}
	}
	return outMap, nil
}

func (r *SampleRunner) computeSummarySimilarities(
	ctx context.Context,
	uniqueIDs []string,
	pairs []SelectedPair,
	summaries map[string]string,
) (map[PairKey]float64, error) {
	var toEmbedIDs []string
	var toEmbedTexts []string
	for _, id := range uniqueIDs {
		if text := strings.TrimSpace(summaries[id]); text != "" {
			toEmbedIDs = append(toEmbedIDs, id)
			toEmbedTexts = append(toEmbedTexts, text)
		}
	}

	vecs := make(map[string][]float32, len(toEmbedIDs))
	for start := 0; start < len(toEmbedTexts); start += summaryEmbedBatch {
		end := start + summaryEmbedBatch
		if end > len(toEmbedTexts) {
			end = len(toEmbedTexts)
		}
		embs, err := r.encoder.Encode(ctx, toEmbedTexts[start:end])
		if err != nil {
			return nil, fmt.Errorf("encode summary batch: %w", err)
		}
		if len(embs) != end-start {
			return nil, fmt.Errorf("encode summary batch: got %d vectors for %d texts", len(embs), end-start)
		}
		for i, vec := range embs {
			vecs[toEmbedIDs[start+i]] = vec
		}
	}

	out := make(map[PairKey]float64, len(pairs))
	for _, p := range pairs {
		va, okA := vecs[p.ArticleA]
		vb, okB := vecs[p.ArticleB]
		if !okA || !okB {
			continue
		}
		sim, err := CosineSimilarity(va, vb)
		if err != nil {
			return nil, fmt.Errorf("summary cosine similarity (%s, %s): %w", p.ArticleA, p.ArticleB, err)
		}
		out[CanonicalPairKey(p.ArticleA, p.ArticleB)] = sim
	}
	return out, nil
}

func toSimilarityLookup(neighbors map[string][]domain.CandidateSimilarityItem) map[string]map[string]float64 {
	out := make(map[string]map[string]float64, len(neighbors))
	for seed, list := range neighbors {
		m := make(map[string]float64, len(list))
		for _, item := range list {
			m[item.ArticleID] = item.Similarity
		}
		out[seed] = m
	}
	return out
}

func lookupUndirected(lookup map[string]map[string]float64, a, b string) (float64, bool) {
	if sim, ok := lookup[a][b]; ok {
		return sim, true
	}
	if sim, ok := lookup[b][a]; ok {
		return sim, true
	}
	return 0, false
}

func collectUniquePairArticleIDs(pairs []SelectedPair) []string {
	seen := make(map[string]struct{}, len(pairs)*2)
	var ids []string
	for _, p := range pairs {
		for _, id := range []string{p.ArticleA, p.ArticleB} {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
	}
	sort.Strings(ids)
	return ids
}

func resolveMetadata(id string, dbMeta, overrides map[string]domain.RagDocumentMetadata) domain.RagDocumentMetadata {
	meta := dbMeta[id]
	if ov, ok := overrides[id]; ok {
		if ov.Title != "" {
			meta.Title = ov.Title
		}
		if ov.URL != "" {
			meta.URL = ov.URL
		}
	}
	if meta.Title == "" {
		meta.Title = id
	}
	return meta
}
