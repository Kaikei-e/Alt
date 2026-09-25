package internal_article_gateway

import (
	"alt/dataplane/port/internal_article_port"
	"alt/dataplane/port/internal_feed_port"
	"alt/dataplane/port/internal_tag_port"
	"alt/shared/driver/alt_db"
)

func toPortArticle(da *alt_db.InternalArticleWithTags) *internal_article_port.ArticleWithTags {
	return &internal_article_port.ArticleWithTags{
		ID:          da.ID,
		Title:       da.Title,
		Content:     da.Content,
		Tags:        da.Tags,
		CreatedAt:   da.CreatedAt,
		UserID:      da.UserID,
		Language:    da.Language,
		PublishedAt: da.PublishedAt,
	}
}

func toPortArticles(driverArticles []*alt_db.InternalArticleWithTags) []*internal_article_port.ArticleWithTags {
	articles := make([]*internal_article_port.ArticleWithTags, len(driverArticles))
	for i, da := range driverArticles {
		articles[i] = toPortArticle(da)
	}
	return articles
}

func toPortDeletedArticles(driverArticles []*alt_db.InternalDeletedArticle) []*internal_article_port.DeletedArticle {
	articles := make([]*internal_article_port.DeletedArticle, len(driverArticles))
	for i, da := range driverArticles {
		articles[i] = &internal_article_port.DeletedArticle{
			ID:        da.ID,
			DeletedAt: da.DeletedAt,
		}
	}
	return articles
}

func toDriverCreateArticleParams(params internal_article_port.CreateArticleParams) alt_db.CreateArticleParams {
	return alt_db.CreateArticleParams{
		Title:       params.Title,
		URL:         params.URL,
		Content:     params.Content,
		FeedID:      params.FeedID,
		UserID:      params.UserID,
		Language:    params.Language,
		PublishedAt: params.PublishedAt,
	}
}

func toPortArticleContent(ac *alt_db.InternalArticleContent) *internal_article_port.ArticleContent {
	if ac == nil {
		return nil
	}
	return &internal_article_port.ArticleContent{
		ID:      ac.ID,
		Title:   ac.Title,
		Content: ac.Content,
		URL:     ac.URL,
		UserID:  ac.UserID,
	}
}

func toPortFeedURLs(driverFeeds []alt_db.InternalFeedURL) []internal_feed_port.FeedURL {
	feeds := make([]internal_feed_port.FeedURL, len(driverFeeds))
	for i, df := range driverFeeds {
		feeds[i] = internal_feed_port.FeedURL{
			FeedID: df.FeedID,
			URL:    df.URL,
		}
	}
	return feeds
}

func toDriverTagUpsertItems(tags []internal_tag_port.TagItem) []alt_db.TagUpsertItem {
	driverTags := make([]alt_db.TagUpsertItem, len(tags))
	for i, t := range tags {
		driverTags[i] = alt_db.TagUpsertItem{Name: t.Name, Confidence: t.Confidence}
	}
	return driverTags
}

func toDriverBatchUpsertTagItems(items []internal_tag_port.BatchUpsertItem) []alt_db.BatchUpsertTagItem {
	driverItems := make([]alt_db.BatchUpsertTagItem, len(items))
	for i, item := range items {
		driverTags := make([]alt_db.TagUpsertItem, len(item.Tags))
		for j, t := range item.Tags {
			driverTags[j] = alt_db.TagUpsertItem{Name: t.Name, Confidence: t.Confidence}
		}
		driverItems[i] = alt_db.BatchUpsertTagItem{
			ArticleID: item.ArticleID,
			FeedID:    item.FeedID,
			Tags:      driverTags,
		}
	}
	return driverItems
}

func groupTagsByArticleIDs(rows []alt_db.BatchArticleTagRow, articleIDs []string) []internal_tag_port.ArticleTagsByID {
	grouped := make(map[string]*internal_tag_port.ArticleTagsByID, len(articleIDs))
	order := make([]string, 0, len(articleIDs))
	for _, row := range rows {
		entry, ok := grouped[row.ArticleID]
		if !ok {
			entry = &internal_tag_port.ArticleTagsByID{ArticleID: row.ArticleID}
			grouped[row.ArticleID] = entry
			order = append(order, row.ArticleID)
		}
		entry.Tags = append(entry.Tags, internal_tag_port.ArticleTagEntry{
			TagName:    row.TagName,
			Confidence: row.Confidence,
			UpdatedAt:  row.UpdatedAt,
		})
	}

	out := make([]internal_tag_port.ArticleTagsByID, 0, len(order))
	for _, id := range order {
		out = append(out, *grouped[id])
	}
	return out
}

func toPortUntaggedArticles(driverArticles []alt_db.InternalUntaggedArticle) []internal_tag_port.UntaggedArticle {
	articles := make([]internal_tag_port.UntaggedArticle, len(driverArticles))
	for i, da := range driverArticles {
		articles[i] = internal_tag_port.UntaggedArticle{
			ID:        da.ID,
			Title:     da.Title,
			Content:   da.Content,
			UserID:    da.UserID,
			FeedID:    da.FeedID,
			CreatedAt: da.CreatedAt,
		}
	}
	return articles
}

func toPortArticlesWithSummaryResults(driverResults []alt_db.ArticleWithSummaryResult) []*internal_article_port.ArticleWithSummaryResult {
	results := make([]*internal_article_port.ArticleWithSummaryResult, len(driverResults))
	for i, dr := range driverResults {
		results[i] = &internal_article_port.ArticleWithSummaryResult{
			ArticleID:       dr.ArticleID,
			ArticleContent:  dr.ArticleContent,
			ArticleURL:      dr.ArticleURL,
			SummaryID:       dr.SummaryID,
			SummaryJapanese: dr.SummaryJapanese,
			CreatedAt:       dr.CreatedAt,
		}
	}
	return results
}

func toPortUnsummarizedArticles(driverArticles []alt_db.InternalUnsummarizedArticle) []*internal_article_port.UnsummarizedArticle {
	articles := make([]*internal_article_port.UnsummarizedArticle, len(driverArticles))
	for i, da := range driverArticles {
		articles[i] = &internal_article_port.UnsummarizedArticle{
			ID:        da.ID,
			Title:     da.Title,
			Content:   da.Content,
			URL:       da.URL,
			CreatedAt: da.CreatedAt,
			UserID:    da.UserID,
		}
	}
	return articles
}
