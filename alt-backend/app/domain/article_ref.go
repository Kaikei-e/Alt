package domain

import "time"

// ArticleRef is the smallest description of an article that can still be
// rendered: what it is called, where it lives, and when it was published.
//
// It exists for the recall rail's fallback. When knowledge_home_items has not
// caught up with a candidate the rail still wants to show the item rather than
// drop it, and these three columns are all the projection would have supplied.
//
// PublishedAt is a pointer because the column is nullable and the distinction
// survives: an article whose feed carried no date has none, and rendering that
// as the zero time would sort it below everything else forever.
//
// A missing article is represented by a nil *ArticleRef, not by an ArticleRef
// with empty fields. The read this replaces returned an empty title for both
// cases and left every caller to guess.
type ArticleRef struct {
	Title       string
	Link        string
	PublishedAt *time.Time
}
