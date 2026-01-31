import type { GenreProgressInfo } from "$lib/schema/dashboard";

/**
 * Filters genre progress entries to exclude "classification" when other genres are present.
 * The "classification" entry is a global aggregate that should be hidden when per-genre
 * items are being displayed.
 */
export function filterGenreProgress(
	genreProgress: Record<string, GenreProgressInfo>,
): [string, GenreProgressInfo][] {
	const entries = Object.entries(genreProgress);

	// Filter out "classification" when other genres are present
	const filtered = entries.filter(([genre]) => {
		if (genre === "classification" && entries.length > 1) {
			return false;
		}
		return true;
	});

	// Sort alphabetically by genre name
	return filtered.sort(([a], [b]) => a.localeCompare(b));
}
