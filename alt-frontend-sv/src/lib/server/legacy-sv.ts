export function buildLegacySvRedirect(
	targetPath: string,
	searchParams: URLSearchParams,
): string {
	const query = searchParams.toString();
	return query ? `${targetPath}?${query}` : targetPath;
}
