export interface OPMLImportResult {
	total: number;
	imported: number;
	skipped: number;
	failed: number;
	failed_urls?: string[];
}
