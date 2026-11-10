<script lang="ts">
import type { TagCloudItem } from "#lib/connect/index.js";

interface Props {
	tags: TagCloudItem[];
	selectedTag: string | null;
	onSelectTag: (tagName: string) => void;
}

let { tags: _tags, selectedTag: _selectedTag, onSelectTag: _onSelectTag }: Props =
	$props();
</script>

<div data-testid="tag-index-stub"></div>
