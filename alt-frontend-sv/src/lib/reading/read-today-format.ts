const HOUR_MS = 60 * 60 * 1000;

export function formatTimeLeft(
	expiresAt: Date | null | undefined,
	now: Date = new Date(),
): string {
	if (!expiresAt) return "";
	const diffMs = expiresAt.getTime() - now.getTime();
	const hours = Math.floor(diffMs / HOUR_MS);
	if (hours < 1) {
		return "under 1h left";
	}
	return `${hours}h left`;
}

export function sourceLabelOf(
	sourceName: string | null | undefined,
	url: string | null | undefined,
): string {
	const trimmed = sourceName?.trim();
	if (trimmed) return trimmed;
	if (!url) return "";
	try {
		return new URL(url).host;
	} catch {
		return "";
	}
}
