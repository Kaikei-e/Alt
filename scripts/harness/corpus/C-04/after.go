package backendapi

type ArticleProto struct {
	Id string
}

func (a *ArticleProto) GetId() string {
	if a != nil {
		return a.Id
	}
	return ""
}

type ListArticlesResponse struct {
	Articles []*ArticleProto
}

func (r *ListArticlesResponse) GetArticles() []*ArticleProto {
	if r != nil {
		return r.Articles
	}
	return nil
}

func extractArticleIDs(resp *ListArticlesResponse) ([]string, error) {
	// BUG: naive protogetter migration omits nil guard; nil resp now succeeds silently with empty slice.
	ids := make([]string, 0, len(resp.GetArticles()))
	for _, a := range resp.GetArticles() {
		ids = append(ids, a.GetId())
	}
	return ids, nil
}
