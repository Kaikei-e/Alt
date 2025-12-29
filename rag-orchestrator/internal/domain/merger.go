package domain

import "unicode/utf8"

// mergeShortChunks merges consecutive paragraphs that are shorter than MinChunkLength.
// Long paragraphs (>= MinChunkLength) are kept separate.
func mergeShortChunks(paragraphs []string) []string {
	if len(paragraphs) == 0 {
		return paragraphs
	}

	var merged []string
	var shortAccumulator string

	for _, para := range paragraphs {
		paraLen := utf8.RuneCountInString(para)
		// If this paragraph is long enough on its own
		if paraLen >= MinChunkLength {
			// Flush any accumulated short paragraphs first
			if shortAccumulator != "" {
				accumLen := utf8.RuneCountInString(shortAccumulator)
				// If still too short, merge with previous or current chunk
				if accumLen < MinChunkLength {
					if len(merged) > 0 {
						// Merge with previous chunk
						lastIdx := len(merged) - 1
						merged[lastIdx] = merged[lastIdx] + "\n\n" + shortAccumulator
					} else {
						// No previous chunk, prepend to current long paragraph
						para = shortAccumulator + "\n\n" + para
					}
				} else {
					merged = append(merged, shortAccumulator)
				}
				shortAccumulator = ""
			}
			// Add this long paragraph (possibly with prepended short content)
			merged = append(merged, para)
		} else {
			// Accumulate short paragraphs
			if shortAccumulator == "" {
				shortAccumulator = para
			} else {
				shortAccumulator = shortAccumulator + "\n\n" + para
			}
		}
	}

	// Handle remaining short paragraphs
	if shortAccumulator != "" {
		accumLen := utf8.RuneCountInString(shortAccumulator)
		// If still too short and we have previous chunks, merge with last chunk
		if accumLen < MinChunkLength && len(merged) > 0 {
			lastIdx := len(merged) - 1
			merged[lastIdx] = merged[lastIdx] + "\n\n" + shortAccumulator
		} else {
			// Otherwise just add it (might still be short if it's the only content)
			merged = append(merged, shortAccumulator)
		}
	}

	return merged
}

// mergeConsecutiveShortChunks performs a second pass to merge remaining short chunks
// that appear consecutively after the initial merge pass.
func mergeConsecutiveShortChunks(paragraphs []string) []string {
	if len(paragraphs) <= 1 {
		return paragraphs
	}

	var result []string
	for i := 0; i < len(paragraphs); i++ {
		current := paragraphs[i]
		currentLen := utf8.RuneCountInString(current)

		// Look ahead: if current is short and next is also short, merge them
		for i+1 < len(paragraphs) {
			nextLen := utf8.RuneCountInString(paragraphs[i+1])
			if currentLen < MinChunkLength && nextLen < MinChunkLength {
				current = current + "\n\n" + paragraphs[i+1]
				currentLen = utf8.RuneCountInString(current)
				i++
			} else {
				break
			}
		}

		// If still short but there's a next paragraph, prepend to next
		if currentLen < MinChunkLength && i+1 < len(paragraphs) {
			paragraphs[i+1] = current + "\n\n" + paragraphs[i+1]
			continue
		}

		// If still short and there's a previous result, append to it
		if currentLen < MinChunkLength && len(result) > 0 {
			result[len(result)-1] = result[len(result)-1] + "\n\n" + current
			continue
		}

		result = append(result, current)
	}
	return result
}
