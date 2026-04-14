<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import {
	createClientTransport,
	fetchTagCloud,
	type TagCloudItem,
} from "$lib/connect";
import { detectGPUBackend } from "$lib/utils/gpuCapability";
import TagVerseScene from "./TagVerseScene.svelte";
import TagVerseHUD from "./TagVerseHUD.svelte";
import { Loader2, AlertCircle, MonitorX } from "@lucide/svelte";

let tags = $state<TagCloudItem[]>([]);
let isLoading = $state(true);
let error = $state<string | null>(null);
let gpuUnsupported = $state(false);
let selectedTag = $state<string | null>(null);

const selectedTagData = $derived(
	selectedTag ? tags.find((t) => t.tagName === selectedTag) : null,
);

onMount(async () => {
	if (!browser) return;
	if (detectGPUBackend() === "none") {
		gpuUnsupported = true;
		isLoading = false;
		return;
	}
	try {
		const transport = createClientTransport();
		tags = await fetchTagCloud(transport, 300);
	} catch (e) {
		error = e instanceof Error ? e.message : "Failed to load tags";
	} finally {
		isLoading = false;
	}
});
</script>

<div class="relative w-full h-[100dvh] bg-black overflow-hidden">
	{#if isLoading}
		<!-- Loading Screen -->
		<div class="flex flex-col items-center justify-center h-full gap-4">
			<Loader2 class="h-8 w-8 animate-spin text-cyan-400" />
			<p class="text-white/60 text-sm">Loading Tag Verse...</p>
		</div>
	{:else if gpuUnsupported}
		<!-- GPU Unsupported Screen -->
		<div class="flex flex-col items-center justify-center h-full gap-4 max-w-md mx-auto text-center">
			<MonitorX class="h-10 w-10 text-amber-400" />
			<p class="text-white/70 text-sm">
				Tag Verse requires WebGPU or WebGL2, which are not available in this browser.
			</p>
			<p class="text-white/40 text-xs">
				Try updating your browser or GPU drivers. On Windows ARM devices, enable
				<code class="bg-white/10 px-1 rounded">chrome://flags/#enable-unsafe-webgpu</code>.
			</p>
		</div>
	{:else if error}
		<!-- Error Screen -->
		<div class="flex flex-col items-center justify-center h-full gap-4">
			<AlertCircle class="h-8 w-8 text-red-400" />
			<p class="text-white/60 text-sm">{error}</p>
			<button
				type="button"
				onclick={() => window.location.reload()}
				class="rounded-lg border border-white/20 px-4 py-2 text-sm text-white/70 hover:bg-white/10 transition-colors"
			>
				Retry
			</button>
		</div>
	{:else if tags.length === 0}
		<!-- Empty State -->
		<div class="flex flex-col items-center justify-center h-full gap-4">
			<p class="text-white/40 text-sm">No tags found. Start adding articles to populate the tag cloud.</p>
		</div>
	{:else}
		<!-- 3D Scene -->
		<TagVerseScene {tags} onTagSelect={(name) => (selectedTag = name)} />

		<!-- Instructions overlay -->
		<div class="absolute bottom-6 left-6 text-white/30 text-xs select-none pointer-events-none">
			<p>Scroll to zoom · Drag to orbit · Click sphere to explore</p>
		</div>

		<!-- HUD Panel -->
		{#if selectedTag && selectedTagData}
			<TagVerseHUD
				tagName={selectedTag}
				articleCount={selectedTagData.articleCount}
				onClose={() => (selectedTag = null)}
			/>
		{/if}
	{/if}
</div>
