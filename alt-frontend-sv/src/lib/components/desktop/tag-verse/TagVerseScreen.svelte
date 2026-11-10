<script lang="ts">
import { onMount } from "svelte";
import { MediaQuery } from "svelte/reactivity";
import {
	createClientTransport,
	fetchTagCloud,
	type TagCloudItem,
} from "#lib/connect/index.js";
import { isWebGL2Available } from "#lib/utils/gpuCapability.js";
import { browser } from "$app/env";
import TagIndexPanel from "./TagIndexPanel.svelte";
import TagVerseHUD from "./TagVerseHUD.svelte";
import TagVerseToolbar from "./TagVerseToolbar.svelte";
import { sumTagArticleCounts } from "./tagIndex.js";

type TagVerseSceneComponent = typeof import("./TagVerseScene.svelte").default;

let tags = $state<TagCloudItem[]>([]);
let SceneComponent = $state<TagVerseSceneComponent | null>(null);
let isLoading = $state(true);
let error = $state<string | null>(null);
let gpuUnsupported = $state(false);
let selectedTag = $state<string | null>(null);
let autoRotate = $state(true);
let resetViewNonce = $state(0);

const reducedMotionQuery = new MediaQuery(
	"prefers-reduced-motion: reduce",
	false,
);
const reducedMotion = $derived(reducedMotionQuery.current);
const effectiveAutoRotate = $derived(reducedMotion ? false : autoRotate);

$effect(() => {
	if (reducedMotion && autoRotate) {
		autoRotate = false;
	}
});

const selectedTagData = $derived(
	selectedTag ? tags.find((t) => t.tagName === selectedTag) : null,
);
const totalArticles = $derived(sumTagArticleCounts(tags));

let cancelCurrentLoad: (() => void) | null = null;

function loadTagVerse() {
	cancelCurrentLoad?.();
	if (!isWebGL2Available()) {
		gpuUnsupported = true;
		isLoading = false;
		return;
	}

	gpuUnsupported = false;
	isLoading = true;
	error = null;

	let cancelled = false;
	cancelCurrentLoad = () => {
		cancelled = true;
	};

	(async () => {
		try {
			const transport = createClientTransport();
			const [fetchedTags, sceneModule] = await Promise.all([
				fetchTagCloud(transport, 300),
				import("./TagVerseScene.svelte"),
			]);
			if (!cancelled) {
				tags = fetchedTags;
				SceneComponent = sceneModule.default;
			}
		} catch (e) {
			if (!cancelled) {
				error = e instanceof Error ? e.message : "Failed to load tags";
			}
		} finally {
			if (!cancelled) {
				isLoading = false;
			}
		}
	})();
}

onMount(() => {
	if (!browser) return;
	loadTagVerse();
	return () => {
		cancelCurrentLoad?.();
	};
});
</script>

<div
	class="flex h-[100dvh] w-full flex-col overflow-hidden bg-[var(--app-bg)] text-[var(--text-primary)]"
>
	<TagVerseToolbar
		tagCount={tags.length}
		{totalArticles}
		autoRotate={effectiveAutoRotate}
		{reducedMotion}
		onToggleRotate={() => {
			if (!reducedMotion) {
				autoRotate = !autoRotate;
			}
		}}
		onResetView={() => {
			resetViewNonce += 1;
		}}
	/>

	<div class="relative flex min-h-0 flex-1 overflow-hidden">
		<TagIndexPanel
			{tags}
			{selectedTag}
			onSelectTag={(name) => {
				selectedTag = name;
			}}
		/>

		<div class="relative h-full min-w-0 flex-1 bg-[var(--app-bg)]">
			{#if isLoading}
				<div class="flex h-full items-center justify-center gap-2.5">
					<span
						aria-hidden="true"
						class="h-2 w-2 bg-[var(--interactive-text)] {reducedMotion
							? ''
							: 'animate-pulse'}"
					></span>
					<p
						class="text-xs text-[var(--text-secondary)]"
						style="font-family: var(--font-mono);"
					>
						Loading tags
					</p>
				</div>
			{:else if gpuUnsupported}
				<div
					class="mx-auto flex h-full max-w-md items-center justify-center px-6 text-center"
				>
					<p class="text-sm text-[var(--text-secondary)]">
						Tag Verse needs WebGL 2, which this browser does not provide.
					</p>
				</div>
			{:else if error}
				<div class="flex h-full flex-col items-center justify-center gap-3 px-6">
					<p class="text-sm text-[var(--alt-error)]">{error}</p>
					<button
						type="button"
						onclick={loadTagVerse}
						class="rounded-none border border-[var(--surface-border)] bg-[var(--action-surface)] px-4 py-1.5 text-xs font-medium text-[var(--text-primary)] transition-colors hover:bg-[var(--action-surface-hover)]"
					>
						Retry
					</button>
				</div>
			{:else if tags.length === 0}
				<div class="flex h-full items-center justify-center px-6 text-center">
					<p class="text-sm text-[var(--text-muted)]">
						No tags yet. Tags appear here once articles are tagged.
					</p>
				</div>
			{:else}
				{#if SceneComponent}
					<SceneComponent
						{tags}
						onTagSelect={(name) => {
							selectedTag = name;
						}}
						{selectedTag}
						autoRotate={effectiveAutoRotate}
						{reducedMotion}
						{resetViewNonce}
					/>
				{/if}

				<div
					class="pointer-events-none absolute bottom-4 left-4 select-none text-[12px] text-[var(--text-muted)]"
				>
					<p>Drag to orbit · Scroll to zoom · Click a tag to open details</p>
				</div>
			{/if}
		</div>

		{#if selectedTag && selectedTagData}
			<TagVerseHUD
				tagName={selectedTag}
				articleCount={selectedTagData.articleCount}
				{reducedMotion}
				onClose={() => {
					selectedTag = null;
				}}
			/>
		{/if}
	</div>
</div>
