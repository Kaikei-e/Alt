package backendapi

import "fmt"

type ArticleProto struct {
	Id string
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
	// Explicit nil check (or direct field dereference) prevents silent empty success on nil response.
	if resp == nil {
		return nil, fmt.Errorf("ListArticlesWithTagsForward: nil response message")
	}
	ids := make([]string, 0, len(resp.Articles))
	for _, a := range resp.Articles {
		ids = append(ids, a.Id)
	}
	return ids, nil
}
