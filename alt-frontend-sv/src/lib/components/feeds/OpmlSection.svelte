<script lang="ts">
import { Download, Upload, FileUp } from "@lucide/svelte";
import { Button } from "$lib/components/ui/button";
import * as Dialog from "$lib/components/ui/dialog";
import { exportOPMLClient, importOPMLClient } from "$lib/api/client";
import type { OPMLImportResult } from "$lib/schema/opml";

interface Props {
	feedCount: number;
	isDesktop: boolean;
	onImportComplete: () => void;
}

const { feedCount, isDesktop, onImportComplete }: Props = $props();

let isExporting = $state(false);
let isImporting = $state(false);
let importResult = $state<OPMLImportResult | null>(null);
let isResultDialogOpen = $state(false);
let errorMessage = $state<string | null>(null);
let dragOver = $state(false);

let fileInput: HTMLInputElement;

async function handleExport() {
	isExporting = true;
	errorMessage = null;
	try {
		const blob = await exportOPMLClient();
		const url = URL.createObjectURL(blob);
		const a = document.createElement("a");
		a.href = url;
		const date = new Date().toISOString().slice(0, 10);
		a.download = `alt-feeds-${date}.opml`;
		document.body.appendChild(a);
		a.click();
		document.body.removeChild(a);
		URL.revokeObjectURL(url);
	} catch (err) {
		errorMessage = err instanceof Error ? err.message : "Export failed";
	} finally {
		isExporting = false;
	}
}

async function handleImportFile(file: File) {
	// Client-side validation
	const validExtensions = [".opml", ".xml"];
	const hasValidExtension = validExtensions.some((ext) =>
		file.name.toLowerCase().endsWith(ext),
	);
	const validMimeTypes = ["text/xml", "application/xml", "text/x-opml"];
	const hasValidMime = validMimeTypes.includes(file.type) || file.type === "";

	if (!hasValidExtension && !hasValidMime) {
		errorMessage = "Please select an OPML or XML file.";
		return;
	}

	if (file.size > 1024 * 1024) {
		errorMessage = "File is too large (max 1MB).";
		return;
	}

	isImporting = true;
	errorMessage = null;
	try {
		const result = await importOPMLClient(file);
		importResult = result;
		isResultDialogOpen = true;
		onImportComplete();
	} catch (err) {
		errorMessage = err instanceof Error ? err.message : "Import failed";
	} finally {
		isImporting = false;
	}
}

function handleFileInputChange(event: Event) {
	const input = event.target as HTMLInputElement;
	const file = input.files?.[0];
	if (file) {
		handleImportFile(file);
		input.value = "";
	}
}

function handleDrop(event: DragEvent) {
	event.preventDefault();
	dragOver = false;
	const file = event.dataTransfer?.files[0];
	if (file) {
		handleImportFile(file);
	}
}

function handleDragOver(event: DragEvent) {
	event.preventDefault();
	dragOver = true;
}

function handleDragLeave() {
	dragOver = false;
}

function handleResultDialogClose(open: boolean) {
	if (!open) {
		isResultDialogOpen = false;
		importResult = null;
	}
}
</script>

<input
	bind:this={fileInput}
	type="file"
	accept=".opml,.xml"
	class="hidden"
	onchange={handleFileInputChange}
	aria-label="Select OPML file"
/>

{#if isDesktop}
	<!-- Desktop Layout -->
	<div
		class="border rounded-lg p-6"
		style="background: var(--surface-bg); border-color: var(--surface-border);"
	>
		<div class="flex items-center justify-between mb-4">
			<div>
				<h2
					class="text-base font-semibold"
					style="color: var(--text-primary);"
				>
					Import / Export
				</h2>
				<p class="text-sm" style="color: var(--text-muted);">
					Import feeds from an OPML file or export your current feeds.
				</p>
			</div>
			<Button
				variant="outline"
				onclick={handleExport}
				disabled={isExporting || feedCount === 0}
			>
				{#if isExporting}
					<span class="flex items-center gap-2">
						<span
							class="animate-spin h-4 w-4 border-2 border-current border-t-transparent rounded-full"
						></span>
						Exporting...
					</span>
				{:else}
					<span class="flex items-center gap-2">
						<Download class="h-4 w-4" />
						Export OPML
					</span>
				{/if}
			</Button>
		</div>

		<!-- Drop Zone -->
		<button
			type="button"
			class="w-full border-2 border-dashed rounded-lg p-8 text-center transition-colors cursor-pointer"
			style="
				border-color: {dragOver ? 'var(--alt-primary)' : 'var(--surface-border)'};
				background: {dragOver ? 'rgba(var(--alt-primary-rgb, 0,0,0), 0.05)' : 'transparent'};
			"
			ondrop={handleDrop}
			ondragover={handleDragOver}
			ondragleave={handleDragLeave}
			onclick={() => fileInput?.click()}
			disabled={isImporting}
			aria-label="Upload OPML file"
		>
			{#if isImporting}
				<div class="flex flex-col items-center gap-2">
					<span
						class="animate-spin h-6 w-6 border-2 border-current border-t-transparent rounded-full"
						style="color: var(--text-muted);"
					></span>
					<span class="text-sm" style="color: var(--text-muted);">
						Importing feeds...
					</span>
				</div>
			{:else}
				<div class="flex flex-col items-center gap-2">
					<FileUp class="h-8 w-8" style="color: var(--text-muted);" />
					<span class="text-sm font-medium" style="color: var(--text-secondary);">
						Drop OPML file here or click to browse
					</span>
					<span class="text-xs" style="color: var(--text-muted);">
						Supports .opml and .xml files (max 1MB)
					</span>
				</div>
			{/if}
		</button>

		{#if errorMessage}
			<div
				class="mt-3 rounded-md p-3 text-sm"
				style="background: var(--alt-error); color: white;"
			>
				{errorMessage}
			</div>
		{/if}
	</div>
{:else}
	<!-- Mobile Layout -->
	<div
		class="rounded-2xl border p-5"
		style="background: var(--surface-bg); border-color: var(--surface-border);"
	>
		<h2
			class="text-sm font-semibold mb-3"
			style="color: var(--text-primary);"
		>
			Import / Export
		</h2>
		<div class="flex gap-3">
			<Button
				class="flex-1 rounded-full min-h-[48px] font-semibold text-sm transition-all duration-200 hover:scale-[1.02] active:scale-[0.98]"
				variant="outline"
				onclick={handleExport}
				disabled={isExporting || feedCount === 0}
			>
				{#if isExporting}
					<span class="flex items-center gap-2">
						<span
							class="animate-spin h-4 w-4 border-2 border-current border-t-transparent rounded-full"
						></span>
						Exporting...
					</span>
				{:else}
					<span class="flex items-center gap-2">
						<Download class="h-4 w-4" />
						Export
					</span>
				{/if}
			</Button>
			<Button
				class="flex-1 rounded-full min-h-[48px] font-semibold text-sm transition-all duration-200 hover:scale-[1.02] active:scale-[0.98]"
				variant="outline"
				onclick={() => fileInput?.click()}
				disabled={isImporting}
			>
				{#if isImporting}
					<span class="flex items-center gap-2">
						<span
							class="animate-spin h-4 w-4 border-2 border-current border-t-transparent rounded-full"
						></span>
						Importing...
					</span>
				{:else}
					<span class="flex items-center gap-2">
						<Upload class="h-4 w-4" />
						Import
					</span>
				{/if}
			</Button>
		</div>

		{#if errorMessage}
			<div
				class="mt-3 rounded-md p-3 text-xs"
				style="background: var(--alt-error); color: white;"
			>
				{errorMessage}
			</div>
		{/if}
	</div>
{/if}

<!-- Import Result Dialog -->
<Dialog.Root open={isResultDialogOpen} onOpenChange={handleResultDialogClose}>
	<Dialog.Portal>
		<Dialog.Overlay />
		<Dialog.Content class="max-w-md">
			<Dialog.Header>
				<Dialog.Title>OPML Import Results</Dialog.Title>
				<Dialog.Description>
					{#if importResult}
						<div class="mt-4 space-y-3">
							<div class="flex justify-between items-center py-2 border-b" style="border-color: var(--surface-border);">
								<span style="color: var(--text-secondary);">Total feeds found</span>
								<span class="font-semibold" style="color: var(--text-primary);">{importResult.total}</span>
							</div>
							<div class="flex justify-between items-center py-2 border-b" style="border-color: var(--surface-border);">
								<span style="color: var(--alt-success);">Imported</span>
								<span class="font-semibold" style="color: var(--alt-success);">{importResult.imported}</span>
							</div>
							<div class="flex justify-between items-center py-2 border-b" style="border-color: var(--surface-border);">
								<span style="color: var(--text-muted);">Already existed</span>
								<span class="font-semibold" style="color: var(--text-muted);">{importResult.skipped}</span>
							</div>
							{#if importResult.failed > 0}
								<div class="flex justify-between items-center py-2 border-b" style="border-color: var(--surface-border);">
									<span style="color: var(--alt-error);">Failed</span>
									<span class="font-semibold" style="color: var(--alt-error);">{importResult.failed}</span>
								</div>
								{#if importResult.failed_urls && importResult.failed_urls.length > 0}
									<div class="mt-2">
										<p class="text-xs font-medium mb-1" style="color: var(--text-secondary);">Failed URLs:</p>
										<div class="max-h-32 overflow-y-auto">
											{#each importResult.failed_urls as url}
												<p class="text-xs truncate py-0.5" style="color: var(--alt-error);" title={url}>
													{url}
												</p>
											{/each}
										</div>
									</div>
								{/if}
							{/if}
						</div>
					{/if}
				</Dialog.Description>
			</Dialog.Header>
			<Dialog.Footer class="mt-4">
				<Button onclick={() => handleResultDialogClose(false)}>
					Close
				</Button>
			</Dialog.Footer>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
