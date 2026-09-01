package eval

// stageAccumulator collects per-stage scores across cases. Each stage counts
// only the cases that can score it — a case without article-level ground truth
// says nothing about retrieval, and averaging a zero in would make an unscored
// case look like a failed one.
type stageAccumulator struct {
	retrievalCases   int
	recallAt5        float64
	recallAt10       float64
	recallAt20       float64
	ndcgAt10         float64
	reciprocalRank   float64
	bm25ZeroCases    int
	forbiddenRetHits int
	forbiddenCases   int

	rerankCases  int
	rerankBefore float64
	rerankAfter  float64
	appliedCases int

	generationCases      int
	faithfulness         float64
	faithfulnessCases    int
	citationCorrectness  float64
	citationCorrectCases int
	citationRecall       float64
	citationRecallCases  int
	forbiddenCiteHits    int
	forbiddenCiteCases   int
	noAnswerHonestCases  int
	noAnswerCases        int
	fallbackCases        int
}

func newStageAccumulator() *stageAccumulator {
	return &stageAccumulator{}
}

func (a *stageAccumulator) observe(gc GoldenCase, result EvalResult) {
	a.observeRetrieval(gc, result)
	a.observeRerank(gc, result)
	a.observeGeneration(gc, result)
}

func (a *stageAccumulator) observeRetrieval(gc GoldenCase, result EvalResult) {
	relevant := gc.Expected.RelevantArticleIDs
	if len(relevant) == 0 {
		return
	}

	a.retrievalCases++
	a.recallAt5 += RecallAtKByID(relevant, result.RetrievedArticleIDs, 5)
	a.recallAt10 += RecallAtKByID(relevant, result.RetrievedArticleIDs, 10)
	a.recallAt20 += RecallAtKByID(relevant, result.RetrievedArticleIDs, 20)
	a.ndcgAt10 += NDCGAtK(gc.Expected.RelevanceGrades(), result.RetrievedArticleIDs, 10)
	a.reciprocalRank += ReciprocalRankByID(relevant, result.RetrievedArticleIDs)
	if result.BM25HitCount == 0 {
		a.bm25ZeroCases++
	}

	if len(gc.Expected.ForbiddenArticleIDs) > 0 {
		a.forbiddenCases++
		if len(ForbiddenHits(result.RetrievedArticleIDs, gc.Expected.ForbiddenArticleIDs)) > 0 {
			a.forbiddenRetHits++
		}
	}
}

func (a *stageAccumulator) observeRerank(gc GoldenCase, result EvalResult) {
	if !result.RerankApplied || len(gc.Expected.RelevantArticleIDs) == 0 || len(result.PreRerankArticleIDs) == 0 {
		return
	}

	grades := gc.Expected.RelevanceGrades()
	a.rerankCases++
	a.appliedCases++
	a.rerankBefore += NDCGAtK(grades, result.PreRerankArticleIDs, 10)
	a.rerankAfter += NDCGAtK(grades, result.RetrievedArticleIDs, 10)
}

func (a *stageAccumulator) observeGeneration(gc GoldenCase, result EvalResult) {
	a.generationCases++

	if result.IsFallback {
		a.fallbackCases++
	}

	if len(gc.Expected.ExpectedEntities) > 0 && result.Answer != "" {
		a.faithfulness += Faithfulness(result.Answer, result.RetrievedTitles, gc.Expected.ExpectedEntities)
		a.faithfulnessCases++
	}

	if len(result.CitedTitles) > 0 && len(gc.Expected.ExpectedTopicKeywords) > 0 {
		a.citationCorrectness += CitationCorrectness(result.CitedTitles, gc.Expected.ExpectedTopicKeywords)
		a.citationCorrectCases++
	}

	if len(gc.Expected.ExpectedCitationArticleIDs) > 0 {
		a.citationRecall += CitationRecallByID(gc.Expected.ExpectedCitationArticleIDs, result.CitedArticleIDs)
		a.citationRecallCases++
	}

	if len(gc.Expected.ForbiddenArticleIDs) > 0 {
		a.forbiddenCiteCases++
		if len(ForbiddenHits(result.CitedArticleIDs, gc.Expected.ForbiddenArticleIDs)) > 0 {
			a.forbiddenCiteHits++
		}
	}

	if gc.Expected.ExpectNoAnswer {
		a.noAnswerCases++
		if result.CitationCount == 0 {
			a.noAnswerHonestCases++
		}
	}
}

func (a *stageAccumulator) finalize() StageMetrics {
	var m StageMetrics

	m.Retrieval.CaseCount = a.retrievalCases
	if a.retrievalCases > 0 {
		n := float64(a.retrievalCases)
		m.Retrieval.MeanRecallAt5 = a.recallAt5 / n
		m.Retrieval.MeanRecallAt10 = a.recallAt10 / n
		m.Retrieval.MeanRecallAt20 = a.recallAt20 / n
		m.Retrieval.MeanNDCGAt10 = a.ndcgAt10 / n
		m.Retrieval.MeanMRR = a.reciprocalRank / n
		m.Retrieval.BM25ZeroRate = float64(a.bm25ZeroCases) / n
	}
	if a.forbiddenCases > 0 {
		m.Retrieval.ForbiddenHitRate = float64(a.forbiddenRetHits) / float64(a.forbiddenCases)
	}

	m.Rerank.CaseCount = a.rerankCases
	if a.rerankCases > 0 {
		n := float64(a.rerankCases)
		m.Rerank.MeanNDCGAt10Before = a.rerankBefore / n
		m.Rerank.MeanNDCGAt10After = a.rerankAfter / n
		m.Rerank.MeanNDCGAt10Delta = m.Rerank.MeanNDCGAt10After - m.Rerank.MeanNDCGAt10Before
	}
	if a.retrievalCases > 0 {
		m.Rerank.AppliedRate = float64(a.appliedCases) / float64(a.retrievalCases)
	}

	m.Generation.CaseCount = a.generationCases
	if a.generationCases > 0 {
		m.Generation.FallbackRate = float64(a.fallbackCases) / float64(a.generationCases)
	}
	if a.faithfulnessCases > 0 {
		m.Generation.MeanFaithfulness = a.faithfulness / float64(a.faithfulnessCases)
	}
	if a.citationCorrectCases > 0 {
		m.Generation.MeanCitationCorrectness = a.citationCorrectness / float64(a.citationCorrectCases)
	}
	if a.citationRecallCases > 0 {
		m.Generation.CitationRecall = a.citationRecall / float64(a.citationRecallCases)
	}
	if a.forbiddenCiteCases > 0 {
		m.Generation.ForbiddenCitationRate = float64(a.forbiddenCiteHits) / float64(a.forbiddenCiteCases)
	}
	if a.noAnswerCases > 0 {
		m.Generation.NoAnswerHonestyRate = float64(a.noAnswerHonestCases) / float64(a.noAnswerCases)
	}

	return m
}
