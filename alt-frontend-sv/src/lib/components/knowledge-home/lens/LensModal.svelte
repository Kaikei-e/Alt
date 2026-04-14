<script lang="ts">
import * as Dialog from "$lib/components/ui/dialog";
import type { ConnectFeedSource } from "$lib/connect";
import type { LensVersionData } from "$lib/connect/knowledge_home";
import TagCombobox from "./TagCombobox.svelte";
import type { TagSuggestion } from "./TagCombobox.svelte";

interface Props {
	open: boolean;
	version: Omit<LensVersionData, "versionId">;
	availableSources: ConnectFeedSource[];
	availableTags?: TagSuggestion[];
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
	availableTags = [],
	loadingSources = false,
	onOpenChange,
	onSave,
}: Props = $props();

let name = $state("");
let description = $state("");
let queryText = $state("");
let selectedTagNames = $state<string[]>([]);
let timeWindow = $state("7d");
let selectedSourceIds = $state<string[]>([]);
let sourceSearch = $state("");
let saving = $state(false);
let nameBlurred = $state(false);

const nameError = $derived(nameBlurred && !name.trim());

function syncFromVersion(nextVersion: Omit<LensVersionData, "versionId">) {
	queryText = nextVersion.queryText ?? "";
	selectedTagNames = [...nextVersion.tagIds];
	timeWindow = nextVersion.timeWindow || "7d";
	selectedSourceIds = [...nextVersion.sourceIds];
	sourceSearch = "";
	nameBlurred = false;
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
		nameBlurred = true;
		return;
	}
	saving = true;
	try {
		await onSave({
			name: name.trim(),
			description: description.trim(),
			version: {
				queryText: queryText.trim(),
				tagIds: selectedTagNames,
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
	<Dialog.Content class="sm:!max-w-2xl max-h-[85dvh] overflow-y-auto">
		<Dialog.Header>
			<Dialog.Title>Save current view</Dialog.Title>
			<Dialog.Description>
				Save the current Home filters as a reusable lens. Only a name is required — filters below are pre-filled from your current view.
			</Dialog.Description>
		</Dialog.Header>

		<div class="space-y-4 py-2">
			<!-- Essential: Name (required) -->
			<label class="block space-y-1">
				<span class="text-sm text-[var(--text-primary)]">
					Name <span class="text-xs text-[var(--alt-primary)]">*</span>
				</span>
				<input
					bind:value={name}
					class="w-full rounded-xl border px-3 py-2 text-sm outline-none transition-colors {nameError
						? 'border-red-400 bg-red-50/30'
						: 'border-[var(--surface-border)] bg-[var(--surface-hover)]'}"
					placeholder="e.g. AI sources, Rust weekly"
					onblur={() => { nameBlurred = true; }}
				/>
				{#if nameError}
					<span class="text-xs text-red-500">Name is required</span>
				{/if}
			</label>

			<!-- Optional: Description -->
			<label class="block space-y-1">
				<span class="text-sm text-[var(--text-primary)]">
					Description <span class="text-xs text-[var(--text-tertiary)]">(optional)</span>
				</span>
				<textarea
					bind:value={description}
					class="min-h-16 w-full rounded-xl border border-[var(--surface-border)] bg-[var(--surface-hover)] px-3 py-2 text-sm outline-none"
					placeholder="A short note about this view"
				></textarea>
			</label>

			<!-- Filter Criteria Section -->
			<div class="border-t border-[var(--surface-border)] pt-4 mt-2">
				<p class="text-xs text-[var(--text-tertiary)] mb-3">Filter criteria — pre-filled from your current view</p>

				<div class="space-y-4">
					<!-- Search query -->
					<label class="block space-y-1">
						<span class="text-sm text-[var(--text-primary)]">
							Search query <span class="text-xs text-[var(--text-tertiary)]">(optional)</span>
						</span>
						<input
							bind:value={queryText}
							class="w-full rounded-xl border border-[var(--surface-border)] bg-[var(--surface-hover)] px-3 py-2 text-sm outline-none"
							placeholder="agents"
						/>
					</label>

					<!-- Tags (TagCombobox) -->
					<div class="space-y-1">
						<span class="text-sm text-[var(--text-primary)]">
							Tags <span class="text-xs text-[var(--text-tertiary)]">(optional)</span>
						</span>
						<TagCombobox
							selectedTags={selectedTagNames}
							{availableTags}
							placeholder="Search or add tags..."
							onTagsChange={(tags) => { selectedTagNames = tags; }}
						/>
					</div>

					<!-- Sources -->
					<div class="space-y-2">
						<div class="flex items-center justify-between gap-3">
							<span class="text-sm text-[var(--text-primary)]">
								Sources <span class="text-xs text-[var(--text-tertiary)]">(optional)</span>
							</span>
							<div class="flex items-center gap-2">
								<span class="text-xs text-[var(--text-secondary)]">{selectedSourceIds.length} of {availableSources.length}</span>
								{#if availableSources.length > 0}
									<button type="button" class="text-xs text-[var(--alt-primary)] hover:underline" onclick={() => { selectedSourceIds = availableSources.map(s => s.id); }}>All</button>
									<button type="button" class="text-xs text-[var(--text-tertiary)] hover:underline" onclick={() => { selectedSourceIds = []; }}>Clear</button>
								{/if}
							</div>
						</div>

						<input
							bind:value={sourceSearch}
							class="w-full rounded-xl border border-[var(--surface-border)] bg-[var(--surface-hover)] px-3 py-2 text-sm outline-none"
							placeholder="Filter sources..."
						/>

						<div class="max-h-48 space-y-1 overflow-y-auto rounded-2xl border border-[var(--surface-border)] bg-[var(--surface-2)] p-2">
							{#if loadingSources}
								<p class="px-2 py-1.5 text-sm text-[var(--text-secondary)]">Loading sources...</p>
							{:else if availableSources.length === 0}
								<p class="px-2 py-1.5 text-sm text-[var(--text-secondary)]">No sources available. Add feeds in Settings → Feeds.</p>
							{:else if filteredSources.length === 0}
								<p class="px-2 py-1.5 text-sm text-[var(--text-secondary)]">No sources match "{sourceSearch}"</p>
							{:else}
								{#each filteredSources as source (source.id)}
									<label class="flex cursor-pointer items-start gap-3 rounded-xl border border-transparent px-2 py-1.5 transition-colors hover:border-[var(--surface-border)]">
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

					<!-- Recent window -->
					<label class="block space-y-1">
						<span class="text-sm text-[var(--text-primary)]">
							Recent window <span class="text-xs text-[var(--text-tertiary)]">(optional)</span>
						</span>
						<select
							bind:value={timeWindow}
							class="w-full rounded-xl border border-[var(--surface-border)] bg-[var(--surface-hover)] px-3 py-2 text-sm outline-none"
						>
							<option value="7d">Last 7 days</option>
							<option value="30d">Last 30 days</option>
							<option value="90d">Last 90 days</option>
						</select>
					</label>
				</div>
			</div>
		</div>

		<div class="mt-4 flex justify-end gap-2 border-t border-[var(--surface-border)] pt-4">
			<button
				class="rounded-none border-2 border-[var(--surface-border)] bg-[var(--surface-bg)] px-4 py-2 text-sm font-bold text-[var(--text-primary)] shadow-[var(--shadow-sm)] transition-all hover:bg-[var(--surface-hover)]"
				onclick={() => onOpenChange(false)}
			>
				Cancel
			</button>
			<button
				class="rounded-none border-2 border-[var(--alt-primary)] bg-[var(--surface-bg)] px-4 py-2 text-sm font-bold text-[var(--text-primary)] shadow-[var(--shadow-sm)] transition-all hover:bg-[var(--alt-primary)] hover:text-white disabled:pointer-events-none disabled:opacity-60"
				onclick={submit}
				disabled={saving || !name.trim()}
			>
				{saving ? "Saving..." : "Save lens"}
			</button>
		</div>
	</Dialog.Content>
</Dialog.Root>
