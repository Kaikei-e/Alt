<script lang="ts">
import * as Dialog from "$lib/components/ui/dialog";
import type { ConnectFeedSource } from "$lib/connect";
import type { LensVersionData } from "$lib/connect/knowledge_home";

interface Props {
	open: boolean;
	version: Omit<LensVersionData, "versionId">;
	availableSources: ConnectFeedSource[];
	loadingSources?: boolean;
	onOpenChange: (open: boolean) => void;
	onSave: (payload: {
		name: string;
		description: string;
		version: Omit<LensVersionData, "versionId">;
	}) => Promise<void> | void;
}

const {
	open,
	version,
	availableSources,
	loadingSources = false,
	onOpenChange,
	onSave,
}: Props = $props();

let name = $state("");
let description = $state("");
let queryText = $state("");
let tagInput = $state("");
let timeWindow = $state("7d");
let selectedSourceIds = $state<string[]>([]);
let sourceSearch = $state("");
let saving = $state(false);

function parseCsv(value: string): string[] {
	return value
		.split(",")
		.map((entry) => entry.trim())
		.filter(Boolean);
}

function syncFromVersion(nextVersion: Omit<LensVersionData, "versionId">) {
	queryText = nextVersion.queryText ?? "";
	tagInput = nextVersion.tagIds.join(", ");
	timeWindow = nextVersion.timeWindow || "7d";
	selectedSourceIds = [...nextVersion.sourceIds];
	sourceSearch = "";
}

$effect(() => {
	if (open) {
		syncFromVersion(version);
	}
});

function displaySourceName(source: ConnectFeedSource): string {
	if (source.title.trim()) {
		return source.title.trim();
	}
	try {
		return new URL(source.url).hostname;
	} catch {
		return source.url;
	}
}

function toggleSource(sourceId: string) {
	selectedSourceIds = selectedSourceIds.includes(sourceId)
		? selectedSourceIds.filter((id) => id !== sourceId)
		: [...selectedSourceIds, sourceId];
}

const filteredSources = $derived.by(() => {
	const query = sourceSearch.trim().toLowerCase();
	return availableSources.filter((source) => {
		const haystack = `${displaySourceName(source)} ${source.url}`.toLowerCase();
		return query ? haystack.includes(query) : true;
	});
});

async function submit() {
	if (!name.trim()) {
		return;
	}
	saving = true;
	try {
		await onSave({
			name: name.trim(),
			description: description.trim(),
			version: {
				queryText: queryText.trim(),
				tagIds: parseCsv(tagInput),
				sourceIds: selectedSourceIds,
				timeWindow,
				includeRecap: version.includeRecap,
				includePulse: version.includePulse,
				sortMode: version.sortMode || "relevance",
			},
		});
		name = "";
		description = "";
		onOpenChange(false);
	} finally {
		saving = false;
	}
}
</script>

<Dialog.Root {open} {onOpenChange}>
	<Dialog.Content class="sm:!max-w-2xl">
		<Dialog.Header>
			<Dialog.Title>Save current view</Dialog.Title>
			<Dialog.Description>
				Save the current Home search, tags, sources, and recent window as a reusable server-side lens.
			</Dialog.Description>
		</Dialog.Header>

		<div class="space-y-4 py-2">
			<label class="block space-y-1">
				<span class="text-sm text-[var(--text-primary)]">Name</span>
				<input bind:value={name} class="w-full rounded-xl border border-[var(--surface-border)] bg-[var(--surface-hover)] px-3 py-2 text-sm outline-none" placeholder="AI sources" />
			</label>

			<label class="block space-y-1">
				<span class="text-sm text-[var(--text-primary)]">Description</span>
				<textarea bind:value={description} class="min-h-20 w-full rounded-xl border border-[var(--surface-border)] bg-[var(--surface-hover)] px-3 py-2 text-sm outline-none" placeholder="Optional note for this view"></textarea>
			</label>

			<label class="block space-y-1">
				<span class="text-sm text-[var(--text-primary)]">Search query</span>
				<input bind:value={queryText} class="w-full rounded-xl border border-[var(--surface-border)] bg-[var(--surface-hover)] px-3 py-2 text-sm outline-none" placeholder="agents" />
				<span class="text-xs text-[var(--text-secondary)]">Saved as a server-side filter for this lens</span>
			</label>

			<label class="block space-y-1">
				<span class="text-sm text-[var(--text-primary)]">Tags</span>
				<input bind:value={tagInput} class="w-full rounded-xl border border-[var(--surface-border)] bg-[var(--surface-hover)] px-3 py-2 text-sm outline-none" placeholder="AI, Rust, Agents" />
				<span class="text-xs text-[var(--text-secondary)]">Comma-separated tag names</span>
			</label>

			<div class="space-y-2">
				<div class="flex items-center justify-between gap-3">
					<span class="text-sm text-[var(--text-primary)]">Sources</span>
					<span class="text-xs text-[var(--text-secondary)]">{selectedSourceIds.length} selected</span>
				</div>

				<input bind:value={sourceSearch} class="w-full rounded-xl border border-[var(--surface-border)] bg-[var(--surface-hover)] px-3 py-2 text-sm outline-none" placeholder="Filter subscribed sources" />

				<div class="max-h-64 space-y-2 overflow-y-auto rounded-2xl border border-[var(--surface-border)] bg-[var(--surface-2)] p-3">
					{#if loadingSources}
						<p class="text-sm text-[var(--text-secondary)]">Loading subscribed sources...</p>
					{:else if filteredSources.length === 0}
						<p class="text-sm text-[var(--text-secondary)]">No subscribed sources match this filter.</p>
					{:else}
						{#each filteredSources as source (source.id)}
							<label class="flex cursor-pointer items-start gap-3 rounded-xl border border-transparent px-2 py-2 transition-colors hover:border-[var(--surface-border)]">
								<input
									type="checkbox"
									class="mt-0.5"
									checked={selectedSourceIds.includes(source.id)}
									onchange={() => toggleSource(source.id)}
								/>
								<span class="min-w-0">
									<span class="block text-sm text-[var(--text-primary)]">{displaySourceName(source)}</span>
									<span class="block truncate text-xs text-[var(--text-secondary)]">{source.url}</span>
								</span>
							</label>
						{/each}
					{/if}
				</div>
			</div>

			<label class="block space-y-1">
				<span class="text-sm text-[var(--text-primary)]">Recent window</span>
				<select bind:value={timeWindow} class="w-full rounded-xl border border-[var(--surface-border)] bg-[var(--surface-hover)] px-3 py-2 text-sm outline-none">
					<option value="7d">Last 7 days</option>
					<option value="30d">Last 30 days</option>
					<option value="90d">Last 90 days</option>
				</select>
			</label>
		</div>

		<div class="mt-4 flex justify-end gap-2">
			<button class="rounded-full border border-[var(--surface-border)] px-4 py-2 text-sm" onclick={() => onOpenChange(false)}>
				Cancel
			</button>
			<button class="rounded-full bg-[var(--accent-primary)] px-4 py-2 text-sm text-white disabled:opacity-50" onclick={submit} disabled={saving || !name.trim()}>
				{saving ? "Saving..." : "Save lens"}
			</button>
		</div>
	</Dialog.Content>
</Dialog.Root>
