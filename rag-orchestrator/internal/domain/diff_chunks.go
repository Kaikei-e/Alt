package domain

// ChunkEventType defines the type of change for a chunk.
type ChunkEventType string

const (
	ChunkEventAdded     ChunkEventType = "added"
	ChunkEventUpdated   ChunkEventType = "updated"
	ChunkEventDeleted   ChunkEventType = "deleted"
	ChunkEventUnchanged ChunkEventType = "unchanged"
)

// ChunkEvent represents a change event for a chunk.
// It maps an optional old chunk to an optional new chunk.
type ChunkEvent struct {
	Type     ChunkEventType
	OldChunk *Chunk // Present for Deleted, Updated, Unchanged
	NewChunk *Chunk // Present for Added, Updated, Unchanged
}

// DiffChunks computes the difference between two lists of chunks.
// It uses a Longest Common Subsequence (LCS) based approach to identify
// unchanged chunks, and heuristically identifies updates.
func DiffChunks(oldChunks, newChunks []Chunk) []ChunkEvent {
	lcs := computeLCS(oldChunks, newChunks)

	var events []ChunkEvent
	oldIdx, newIdx := 0, 0

	for _, match := range lcs {
		// Process gaps before the match

		// Collect gap chunks
		var gapOld []Chunk
		for oldIdx < match.oldIdx {
			gapOld = append(gapOld, oldChunks[oldIdx])
			oldIdx++
		}

		var gapNew []Chunk
		for newIdx < match.newIdx {
			gapNew = append(gapNew, newChunks[newIdx])
			newIdx++
		}

		// Process the gap
		gapEvents := processGap(gapOld, gapNew)
		events = append(events, gapEvents...)

		// Process the match itself
		events = append(events, ChunkEvent{
			Type:     ChunkEventUnchanged,
			OldChunk: &oldChunks[match.oldIdx],
			NewChunk: &newChunks[match.newIdx],
		})

		oldIdx++
		newIdx++
	}

	// Process trailing gap
	var gapOld []Chunk
	for oldIdx < len(oldChunks) {
		gapOld = append(gapOld, oldChunks[oldIdx])
		oldIdx++
	}

	var gapNew []Chunk
	for newIdx < len(newChunks) {
		gapNew = append(gapNew, newChunks[newIdx])
		newIdx++
	}
	events = append(events, processGap(gapOld, gapNew)...)

	return events
}

type lcsMatch struct {
	oldIdx int
	newIdx int
}

// computeLCS calculates the Longest Common Subsequence of matching chunks.
// Chunks match if their Hash is identical.
func computeLCS(oldChunks, newChunks []Chunk) []lcsMatch {
	n := len(oldChunks)
	m := len(newChunks)

	// dp[i][j] stores the length of LCS for oldChunks[:i] and newChunks[:j]
	// Using a 1D array for optimization if needed, but 2D is clearer for reconstruction.
	// Given chunk lists are small (tens to hundreds), 2D is fine.
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}

	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if oldChunks[i-1].Hash == newChunks[j-1].Hash {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				if dp[i-1][j] > dp[i][j-1] {
					dp[i][j] = dp[i-1][j]
				} else {
					dp[i][j] = dp[i][j-1]
				}
			}
		}
	}

	// Backtrack to find the LCS
	var matches []lcsMatch
	i, j := n, m
	for i > 0 && j > 0 {
		if oldChunks[i-1].Hash == newChunks[j-1].Hash {
			matches = append(matches, lcsMatch{oldIdx: i - 1, newIdx: j - 1})
			i--
			j--
		} else if dp[i-1][j] > dp[i][j-1] {
			i--
		} else {
			j--
		}
	}

	// Reverse matches to get correct order
	for k := 0; k < len(matches)/2; k++ {
		matches[k], matches[len(matches)-1-k] = matches[len(matches)-1-k], matches[k]
	}

	return matches
}

// processGap determines events for a mismatch gap.
// Heuristic: If number of items in gap matches, treat as Updates.
// Otherwise, treat as Delete + Add.
func processGap(gapOld, gapNew []Chunk) []ChunkEvent {
	var events []ChunkEvent

	if len(gapOld) > 0 && len(gapOld) == len(gapNew) {
		// Heuristic: 1-to-1 mapping -> Update
		for i := range gapOld {
			events = append(events, ChunkEvent{
				Type:     ChunkEventUpdated,
				OldChunk: &gapOld[i],
				NewChunk: &gapNew[i],
			})
		}
	} else {
		// Mismatch count -> Deletes and Adds
		for i := range gapOld {
			events = append(events, ChunkEvent{
				Type:     ChunkEventDeleted,
				OldChunk: &gapOld[i],
				NewChunk: nil,
			})
		}
		for i := range gapNew {
			events = append(events, ChunkEvent{
				Type:     ChunkEventAdded,
				OldChunk: nil,
				NewChunk: &gapNew[i],
			})
		}
	}

	return events
}
