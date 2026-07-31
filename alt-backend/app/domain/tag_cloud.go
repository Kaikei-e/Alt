package domain

// TagCloudItem represents a tag with its associated article count
// and 3D position for the Tag Verse visualization.
type TagCloudItem struct {
	TagName      string  `json:"tag_name"`
	ArticleCount int     `json:"article_count"`
	PositionX    float64 `json:"position_x"`
	PositionY    float64 `json:"position_y"`
	PositionZ    float64 `json:"position_z"`
}

// TagCooccurrence represents a pair of tags that share articles.
type TagCooccurrence struct {
	TagNameA    string
	TagNameB    string
	SharedCount int
}

// TagArticleCount is how many of one user's articles carry a tag within a
// window.
//
// It lives in the domain rather than in the Knowledge Home port it feeds
// because the count is now read across a process boundary: alt-data-hub owns
// the query, and a type declared inside an orchestrator port would make the
// data plane depend on the consumer that happens to use the numbers today.
// What "trending" means from these counts stays with that consumer.
type TagArticleCount struct {
	TagName      string
	ArticleCount int
}
