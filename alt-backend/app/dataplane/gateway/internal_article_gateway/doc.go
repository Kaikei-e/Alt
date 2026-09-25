// Package internal_article_gateway provides gateway implementations for
// internal article API.
//
// It sits under dataplane/ since ADR-000954. It used to live in
// shared/, which was accurate while both cmd/backend and cmd/datahub built it;
// after moving GetArticleTitleAndLink onto the wire, cmd/datahub is the
// only binary that constructs it, and a package under shared/ holding an
// *alt_db.AltDBRepository is an invitation for the next caller to reach for a
// pool that no longer exists in its process.
package internal_article_gateway
