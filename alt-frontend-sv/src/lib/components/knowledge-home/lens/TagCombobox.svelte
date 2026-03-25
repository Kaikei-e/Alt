<script lang="ts" module>
export interface TagSuggestion {
	name: string;
	count?: number;
}
</script>

<script lang="ts">
import { X } from "@lucide/svelte";

interface Props {
	selectedTags: string[];
	availableTags: TagSuggestion[];
	placeholder?: string;
	onTagsChange: (tags: string[]) => void;
}

const {
	selectedTags,
	availableTags,
	placeholder = "Search or add tags...",
	onTagsChange,
}: Props = $props();

const MAX_SUGGESTIONS = 20;

let query = $state("");
let isOpen = $state(false);
let highlightedIndex = $state(-1);
let inputEl = $state<HTMLInputElement | null>(null);

const filteredSuggestions = $derived.by(() => {
	const q = query.trim().toLowerCase();
	const selectedSet = new Set(selectedTags);
	return availableTags
		.filter((tag) => {
			if (selectedSet.has(tag.name)) return false;
			return q ? tag.name.toLowerCase().includes(q) : true;
		})
		.slice(0, MAX_SUGGESTIONS);
});

const showCreateOption = $derived.by(() => {
	const q = query.trim();
	if (!q) return false;
	const lowerQ = q.toLowerCase();
	const alreadySelected = selectedTags.some(
		(t) => t.toLowerCase() === lowerQ,
	);
	const exactMatch = availableTags.some(
		(t) => t.name.toLowerCase() === lowerQ,
	);
	return !alreadySelected && !exactMatch;
});

const totalOptions = $derived(
	filteredSuggestions.length + (showCreateOption ? 1 : 0),
);

function addTag(tagName: string) {
	const trimmed = tagName.trim();
	if (!trimmed) return;
	if (selectedTags.includes(trimmed)) return;
	onTagsChange([...selectedTags, trimmed]);
	query = "";
	highlightedIndex = -1;
}

function removeTag(tagName: string) {
	onTagsChange(selectedTags.filter((t) => t !== tagName));
}

function handleKeydown(e: KeyboardEvent) {
	if (e.key === "ArrowDown") {
		e.preventDefault();
		if (!isOpen) isOpen = true;
		highlightedIndex = Math.min(highlightedIndex + 1, totalOptions - 1);
	} else if (e.key === "ArrowUp") {
		e.preventDefault();
		highlightedIndex = Math.max(highlightedIndex - 1, 0);
	} else if (e.key === "Enter") {
		e.preventDefault();
		if (highlightedIndex >= 0 && highlightedIndex < filteredSuggestions.length) {
			const suggestion = filteredSuggestions[highlightedIndex];
			if (suggestion) addTag(suggestion.name);
		} else if (
			showCreateOption &&
			highlightedIndex === filteredSuggestions.length
		) {
			addTag(query.trim());
		} else if (showCreateOption && query.trim()) {
			addTag(query.trim());
		}
	} else if (e.key === "Escape") {
		isOpen = false;
		highlightedIndex = -1;
	} else if (e.key === "Backspace" && !query && selectedTags.length > 0) {
		const lastTag = selectedTags[selectedTags.length - 1];
		if (lastTag) removeTag(lastTag);
	}
}

function handleInput() {
	isOpen = true;
	highlightedIndex = -1;
}

function handleFocus() {
	isOpen = true;
}

function handleBlur() {
	setTimeout(() => {
		isOpen = false;
		highlightedIndex = -1;
	}, 150);
}

const listboxId = "tag-combobox-listbox";
</script>

<div class="space-y-1.5">
	<!-- Selected tag chips -->
	{#if selectedTags.length > 0}
		<div class="flex flex-wrap gap-1">
			{#each selectedTags as tag (tag)}
				<span
					class="inline-flex items-center gap-1 rounded border border-[var(--chip-border)] bg-[var(--chip-bg)] px-2 py-0.5 text-xs font-medium text-[var(--chip-text)]"
				>
					{tag}
					<button
						type="button"
						class="inline-flex items-center justify-center rounded-full hover:bg-[var(--surface-hover)] transition-colors"
						aria-label="Remove {tag}"
						onclick={() => removeTag(tag)}
					>
						<X class="h-3 w-3" />
					</button>
				</span>
			{/each}
		</div>
	{/if}

	<!-- Combobox input -->
	<div class="relative">
		<input
			bind:this={inputEl}
			type="text"
			bind:value={query}
			oninput={handleInput}
			onkeydown={handleKeydown}
			onblur={handleBlur}
			onfocus={handleFocus}
			{placeholder}
			class="w-full rounded-xl border border-[var(--surface-border)] bg-[var(--surface-hover)] px-3 py-2 text-sm outline-none"
			role="combobox"
			aria-expanded={isOpen && totalOptions > 0}
			aria-controls={listboxId}
			aria-activedescendant={highlightedIndex >= 0
				? `tag-option-${highlightedIndex}`
				: undefined}
			autocomplete="off"
		/>

		{#if isOpen && totalOptions > 0}
			<ul
				id={listboxId}
				role="listbox"
				class="absolute top-full left-0 z-50 mt-1 w-full max-h-[200px] overflow-y-auto rounded-xl border border-[var(--surface-border)] bg-[var(--surface-bg)] shadow-sm"
			>
				{#each filteredSuggestions as suggestion, index (suggestion.name)}
					<li
						id="tag-option-{index}"
						role="option"
						aria-selected={highlightedIndex === index}
						class="flex cursor-pointer items-center justify-between px-3 py-1.5 text-sm transition-colors {highlightedIndex ===
						index
							? 'bg-[var(--surface-hover)]'
							: 'hover:bg-[var(--surface-hover)]'}"
						onmousedown={() => addTag(suggestion.name)}
						onmouseenter={() => (highlightedIndex = index)}
					>
						<span class="text-[var(--text-primary)]">{suggestion.name}</span>
						{#if suggestion.count != null}
							<span class="text-xs text-[var(--text-tertiary)]"
								>×{suggestion.count}</span
							>
						{/if}
					</li>
				{/each}

				{#if showCreateOption}
					<li
						id="tag-option-{filteredSuggestions.length}"
						role="option"
						aria-selected={highlightedIndex === filteredSuggestions.length}
						class="flex cursor-pointer items-center px-3 py-1.5 text-sm transition-colors {highlightedIndex ===
						filteredSuggestions.length
							? 'bg-[var(--surface-hover)]'
							: 'hover:bg-[var(--surface-hover)]'}"
						onmousedown={() => addTag(query.trim())}
						onmouseenter={() =>
							(highlightedIndex = filteredSuggestions.length)}
					>
						<span class="text-[var(--text-secondary)]"
							>Create "{query.trim()}"</span
						>
					</li>
				{/if}
			</ul>
		{/if}
	</div>
</div>
