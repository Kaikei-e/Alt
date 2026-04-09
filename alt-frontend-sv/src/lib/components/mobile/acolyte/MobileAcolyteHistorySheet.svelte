<script lang="ts">
import type { AcolyteVersionSummary } from "$lib/connect/acolyte";
import * as Sheet from "$lib/components/ui/sheet";
import { X } from "@lucide/svelte";

interface Props {
	open: boolean;
	versions: AcolyteVersionSummary[];
	onClose: () => void;
}

let { open, versions, onClose }: Props = $props();

const CHANGE_ICONS: Record<string, string> = {
	added: "+",
	updated: "~",
	removed: "\u2212",
	regenerated: "\u21BB",
};

const CHANGE_COLORS: Record<string, { bg: string; text: string }> = {
	added: { bg: "#ecfdf5", text: "#065f46" },
	updated: { bg: "#eff6ff", text: "#1e40af" },
	removed: { bg: "#fef2f2", text: "#991b1b" },
	regenerated: { bg: "#fefce8", text: "#854d0e" },
};
</script>

<Sheet.Root bind:open onOpenChange={(value) => !value && onClose()}>
	<Sheet.Content
		side="bottom"
		class="max-h-[85vh] border-t border-[var(--surface-border,#c8c8c8)] shadow-lg w-full max-w-full sm:max-w-full p-0 gap-0 flex flex-col overflow-hidden [&>button.ring-offset-background]:hidden"
		style="background: var(--surface-bg, #faf9f7) !important; border-radius: 0;"
		data-testid="history-sheet"
	>
		<!-- Header -->
		<Sheet.Header class="border-b border-[var(--surface-border,#c8c8c8)] px-4 py-3">
			<div class="flex items-center justify-between">
				<Sheet.Title
					class="font-[var(--font-body)] text-[0.65rem] font-bold uppercase tracking-[0.12em] text-[var(--alt-ash,#999)] m-0"
				>
					Editions
				</Sheet.Title>
			</div>
		</Sheet.Header>

		<!-- Scrollable content -->
		<div class="overflow-y-auto flex-1 px-4 py-3">
			{#if versions.length === 0}
				<p class="font-[var(--font-body)] text-[0.8rem] text-[var(--alt-ash,#999)] italic">
					No versions recorded.
				</p>
			{:else}
				<ol class="list-none p-0 m-0 flex flex-col gap-2">
					{#each versions as ver, i}
						<li
							class="p-3 border border-transparent transition-colors duration-150 active:bg-[var(--surface-hover,#f3f1ed)]"
							style="animation: card-in 0.25s ease forwards; animation-delay: calc({i} * 40ms); opacity: 0;"
						>
							<div class="flex items-center justify-between">
								<span class="font-[var(--font-mono)] text-[0.75rem] font-semibold text-[var(--alt-charcoal,#1a1a1a)]">
									Ed. {ver.versionNo}
								</span>
								<time class="font-[var(--font-body)] text-[0.65rem] text-[var(--alt-ash,#999)]">
									{new Date(ver.createdAt).toLocaleDateString("en-US", { month: "short", day: "numeric" })}
								</time>
							</div>
							{#if ver.changeReason}
								<p class="font-[var(--font-body)] text-[0.75rem] text-[var(--alt-slate,#666)] mt-0.5 mb-0 leading-snug">
									{ver.changeReason}
								</p>
							{/if}
							{#if ver.changeItems?.length > 0}
								<div class="flex flex-wrap gap-1 mt-1.5">
									{#each ver.changeItems as ci}
										{@const colors = CHANGE_COLORS[ci.changeKind] ?? { bg: "#f5f5f5", text: "#666" }}
										<span
											class="inline-flex items-center gap-0.5 font-[var(--font-mono)] text-[0.6rem] px-1.5 py-0.5"
											style="background: {colors.bg}; color: {colors.text};"
										>
											<span class="font-bold">{CHANGE_ICONS[ci.changeKind] ?? "?"}</span>
											{ci.fieldName}
										</span>
									{/each}
								</div>
							{/if}
						</li>
					{/each}
				</ol>
			{/if}
		</div>

		<!-- Footer safe area -->
		<div class="px-4 pb-[calc(1rem+env(safe-area-inset-bottom,0px))]"></div>

		<!-- Close button -->
		<Sheet.Close
			class="absolute right-3 top-3 h-8 w-8 border border-[var(--surface-border,#c8c8c8)] text-[var(--alt-charcoal,#1a1a1a)] transition-colors inline-flex shrink-0 items-center justify-center focus-visible:outline-none active:bg-[var(--surface-hover,#f3f1ed)]"
			style="background: var(--surface-bg, #faf9f7); border-radius: 0;"
			aria-label="Close"
		>
			<X class="h-4 w-4" />
		</Sheet.Close>
	</Sheet.Content>
</Sheet.Root>

<style>
	@keyframes card-in {
		to { opacity: 1; }
	}
</style>
