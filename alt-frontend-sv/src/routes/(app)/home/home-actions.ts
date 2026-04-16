import type { KnowledgeHomeItemData } from "$lib/connect/knowledge_home";

// Never embed article title / excerpt / id in TrackHomeAction's body:
// the server reads only meta.query and meta.tag (see
// track_home_action_usecase.go), and article text in the POST body has
// triggered upstream WAF rules that block specific articles before the
// request reaches our infrastructure.
export function buildHomeActionMetadata(
	_type: string,
	_item: KnowledgeHomeItemData,
): string | undefined {
	return undefined;
}
