<script lang="ts">
import * as Dialog from "$lib/components/ui/dialog";
import type { LensVersionData } from "$lib/connect/knowledge_home";

interface Props {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	onSave: (payload: {
		name: string;
		description: string;
		version: Omit<LensVersionData, "versionId">;
	}) => Promise<void> | void;
}

const { open, onOpenChange, onSave }: Props = $props();

let name = $state("");
let description = $state("");
let tagInput = $state("");
let feedInput = $state("");
let timeWindow = $state("7d");
let saving = $state(false);

function parseCsv(value: string): string[] {
	return value
		.split(",")
		.map((entry) => entry.trim())
		.filter(Boolean);
}

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
				queryText: "",
				tagIds: parseCsv(tagInput),
				feedIds: parseCsv(feedInput),
				timeWindow,
				includeRecap: false,
				includePulse: false,
				sortMode: "relevance",
			},
		});
		name = "";
		description = "";
		tagInput = "";
		feedInput = "";
		timeWindow = "7d";
		onOpenChange(false);
	} finally {
		saving = false;
	}
}
</script>

<Dialog.Root {open} {onOpenChange}>
	<Dialog.Content class="sm:!max-w-lg">
		<Dialog.Header>
			<Dialog.Title>Save Lens</Dialog.Title>
			<Dialog.Description>
				Create a server-side view for the Home stream using tags, feed IDs, and a recent window.
			</Dialog.Description>
		</Dialog.Header>

		<div class="space-y-4 py-2">
			<label class="block space-y-1">
				<span class="text-sm text-[var(--text-primary)]">Name</span>
				<input bind:value={name} class="w-full rounded-xl border border-[var(--surface-border)] bg-[var(--surface-1)] px-3 py-2 text-sm outline-none" placeholder="AI sources" />
			</label>

			<label class="block space-y-1">
				<span class="text-sm text-[var(--text-primary)]">Description</span>
				<textarea bind:value={description} class="min-h-20 w-full rounded-xl border border-[var(--surface-border)] bg-[var(--surface-1)] px-3 py-2 text-sm outline-none" placeholder="Optional note for this view"></textarea>
			</label>

			<label class="block space-y-1">
				<span class="text-sm text-[var(--text-primary)]">Tags</span>
				<input bind:value={tagInput} class="w-full rounded-xl border border-[var(--surface-border)] bg-[var(--surface-1)] px-3 py-2 text-sm outline-none" placeholder="AI, Rust, Agents" />
				<span class="text-xs text-[var(--text-secondary)]">Comma-separated tag names</span>
			</label>

			<label class="block space-y-1">
				<span class="text-sm text-[var(--text-primary)]">Feed IDs</span>
				<input bind:value={feedInput} class="w-full rounded-xl border border-[var(--surface-border)] bg-[var(--surface-1)] px-3 py-2 text-sm outline-none" placeholder="uuid-1, uuid-2" />
				<span class="text-xs text-[var(--text-secondary)]">Comma-separated feed UUIDs</span>
			</label>

			<label class="block space-y-1">
				<span class="text-sm text-[var(--text-primary)]">Recent window</span>
				<select bind:value={timeWindow} class="w-full rounded-xl border border-[var(--surface-border)] bg-[var(--surface-1)] px-3 py-2 text-sm outline-none">
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
