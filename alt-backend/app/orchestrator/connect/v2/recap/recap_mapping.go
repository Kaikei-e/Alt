package recap

import (
	"time"

	recapv2 "alt/gen/proto/alt/recap/v2"

	"alt/domain"
	"alt/utils/safeconv"
)

// domainToProto converts domain.RecapSummary to 7-day proto response.
func domainToProto(recap *domain.RecapSummary) *recapv2.GetSevenDayRecapResponse {
	genres := make([]*recapv2.RecapGenre, len(recap.Genres))
	for i, g := range recap.Genres {
		genres[i] = &recapv2.RecapGenre{
			Genre:         g.Genre,
			Summary:       g.Summary,
			TopTerms:      g.TopTerms,
			ArticleCount:  safeconv.Int32(g.ArticleCount),
			ClusterCount:  safeconv.Int32(g.ClusterCount),
			EvidenceLinks: evidenceLinksToProto(g.EvidenceLinks),
			Bullets:       g.Bullets,
			References:    referencesToProto(g.References),
		}
	}

	return &recapv2.GetSevenDayRecapResponse{
		JobId:         recap.JobID,
		ExecutedAt:    recap.ExecutedAt.Format(time.RFC3339),
		WindowStart:   recap.WindowStart.Format(time.RFC3339),
		WindowEnd:     recap.WindowEnd.Format(time.RFC3339),
		TotalArticles: safeconv.Int32(recap.TotalArticles),
		Genres:        genres,
	}
}

// domainToProtoThreeDays converts domain.RecapSummary to 3-day proto response.
func domainToProtoThreeDays(recap *domain.RecapSummary) *recapv2.GetThreeDayRecapResponse {
	genres := make([]*recapv2.RecapGenre, len(recap.Genres))
	for i, g := range recap.Genres {
		genres[i] = &recapv2.RecapGenre{
			Genre:         g.Genre,
			Summary:       g.Summary,
			TopTerms:      g.TopTerms,
			ArticleCount:  safeconv.Int32(g.ArticleCount),
			ClusterCount:  safeconv.Int32(g.ClusterCount),
			EvidenceLinks: evidenceLinksToProto(g.EvidenceLinks),
			Bullets:       g.Bullets,
			References:    referencesToProto(g.References),
		}
	}

	return &recapv2.GetThreeDayRecapResponse{
		JobId:         recap.JobID,
		ExecutedAt:    recap.ExecutedAt.Format(time.RFC3339),
		WindowStart:   recap.WindowStart.Format(time.RFC3339),
		WindowEnd:     recap.WindowEnd.Format(time.RFC3339),
		TotalArticles: safeconv.Int32(recap.TotalArticles),
		Genres:        genres,
	}
}

// domainToProtoThreeDaysCards converts domain.RecapCardsResponse to proto response.
func domainToProtoThreeDaysCards(resp *domain.RecapCardsResponse) *recapv2.GetThreeDayRecapCardsResponse {
	if resp == nil {
		return &recapv2.GetThreeDayRecapCardsResponse{
			Cards: []*recapv2.RecapCard{},
		}
	}

	protoResp := &recapv2.GetThreeDayRecapCardsResponse{
		Cards: make([]*recapv2.RecapCard, 0, len(resp.Cards)),
	}

	if resp.Job != nil {
		protoResp.Job = &recapv2.RecapCardsJob{
			JobId:         resp.Job.JobID,
			KickedAt:      resp.Job.KickedAt,
			From:          resp.Job.From,
			To:            resp.Job.To,
			ParamsVersion: resp.Job.ParamsVersion,
			CardsSelected: safeconv.Int32(resp.Job.CardsSelected),
			Degraded:      resp.Job.Degraded,
		}
	}

	for _, c := range resp.Cards {
		if c == nil {
			continue
		}
		card := &recapv2.RecapCard{
			Id:              c.ID,
			Rank:            safeconv.Int32(c.Rank),
			StoryId:         c.StoryID,
			ContinuesCardId: c.ContinuesCardID,
			HeadlineJa:      c.HeadlineJa,
			WhatJa:          c.WhatJa,
			WhyJa:           c.WhyJa,
			Genre:           c.Genre,
			CreatedAt:       c.CreatedAt,
			Sources:         make([]*recapv2.RecapCardSource, 0, len(c.Sources)),
		}
		for _, s := range c.Sources {
			if s == nil {
				continue
			}
			card.Sources = append(card.Sources, &recapv2.RecapCardSource{
				N:       safeconv.Int32(s.N),
				FeedId:  s.FeedID,
				Url:     s.URL,
				Host:    s.Host,
				Title:   s.Title,
				PubDate: s.PubDate,
			})
		}
		protoResp.Cards = append(protoResp.Cards, card)
	}

	return protoResp
}

// evidenceLinksToProto converts domain evidence links to proto.
func evidenceLinksToProto(links []domain.EvidenceLink) []*recapv2.EvidenceLink {
	result := make([]*recapv2.EvidenceLink, len(links))
	for i, l := range links {
		result[i] = &recapv2.EvidenceLink{
			ArticleId:   l.ArticleID,
			Title:       l.Title,
			SourceUrl:   l.SourceURL,
			PublishedAt: l.PublishedAt,
			Lang:        l.Lang,
		}
	}
	return result
}

// referencesToProto converts domain references to proto.
func referencesToProto(refs []domain.Reference) []*recapv2.Reference {
	result := make([]*recapv2.Reference, len(refs))
	for i, r := range refs {
		result[i] = &recapv2.Reference{
			Id:     safeconv.Int32(r.ID),
			Url:    r.URL,
			Domain: r.Domain,
		}
		if r.ArticleID != nil {
			result[i].ArticleId = r.ArticleID
		}
	}
	return result
}

// clusterDraftToProto converts domain ClusterDraft to proto.
func clusterDraftToProto(draft *domain.ClusterDraft) *recapv2.ClusterDraft {
	genres := make([]*recapv2.ClusterGenre, len(draft.Genres))
	for i, g := range draft.Genres {
		genres[i] = &recapv2.ClusterGenre{
			Genre:        g.Genre,
			SampleSize:   safeconv.Int32(g.SampleSize),
			ClusterCount: safeconv.Int32(g.ClusterCount),
			Clusters:     clusterSegmentsToProto(g.Clusters),
		}
	}

	return &recapv2.ClusterDraft{
		DraftId:      draft.ID,
		Description:  draft.Description,
		Source:       draft.Source,
		GeneratedAt:  draft.GeneratedAt.Format(time.RFC3339),
		TotalEntries: safeconv.Int32(draft.TotalEntries),
		Genres:       genres,
	}
}

// clusterSegmentsToProto converts domain ClusterSegments to proto.
func clusterSegmentsToProto(segments []domain.ClusterSegment) []*recapv2.ClusterSegment {
	result := make([]*recapv2.ClusterSegment, len(segments))
	for i, s := range segments {
		result[i] = &recapv2.ClusterSegment{
			ClusterId:                s.ClusterID,
			Label:                    s.Label,
			Count:                    safeconv.Int32(s.Count),
			MarginMean:               s.MarginMean,
			MarginStd:                s.MarginStd,
			TopBoostMean:             s.TopBoostMean,
			GraphBoostAvailableRatio: s.GraphBoostAvailableRatio,
			TagCountMean:             s.TagCountMean,
			TagEntropyMean:           s.TagEntropyMean,
			TopTags:                  s.TopTags,
			RepresentativeArticles:   clusterArticlesToProto(s.RepresentativeArticles),
		}
	}
	return result
}

// clusterArticlesToProto converts domain ClusterArticles to proto.
func clusterArticlesToProto(articles []domain.ClusterArticle) []*recapv2.ClusterArticle {
	result := make([]*recapv2.ClusterArticle, len(articles))
	for i, a := range articles {
		result[i] = &recapv2.ClusterArticle{
			ArticleId:      a.ArticleID,
			Margin:         a.Margin,
			TopBoost:       a.TopBoost,
			Strategy:       a.Strategy,
			TagCount:       safeconv.Int32(a.TagCount),
			CandidateCount: safeconv.Int32(a.CandidateCount),
			TopTags:        a.TopTags,
		}
	}
	return result
}

// recapSearchResultsToProto converts domain search results to proto items.
func recapSearchResultsToProto(results []*domain.RecapSearchResult) []*recapv2.RecapSearchResultItem {
	protoResults := make([]*recapv2.RecapSearchResultItem, len(results))
	for i, r := range results {
		protoResults[i] = &recapv2.RecapSearchResultItem{
			JobId:      r.JobID,
			ExecutedAt: r.ExecutedAt,
			WindowDays: safeconv.Int32(r.WindowDays),
			Genre:      r.Genre,
			Summary:    r.Summary,
			TopTerms:   r.TopTerms,
			Bullets:    r.Bullets,
		}
	}
	return protoResults
}
