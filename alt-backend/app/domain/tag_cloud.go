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
