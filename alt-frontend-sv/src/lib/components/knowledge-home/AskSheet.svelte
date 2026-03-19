<script lang="ts">
interface Props {
	open: boolean;
	scopeTitle: string;
	scopeContext?: string;
	onClose: () => void;
	onSubmit: (question: string) => void;
}

const { open, scopeTitle, scopeContext, onClose, onSubmit }: Props = $props();

let question = $state("");

const suggestions = [
	"What is the key point?",
	"What is new here?",
	"What should I read next?",
] as const;

function submit() {
	const trimmed = question.trim();
	if (!trimmed) return;
	onSubmit(trimmed);
}
</script>

{#if open}
	<div class="fixed inset-0 z-40 bg-black/40" role="presentation" onclick={onClose}></div>
	<div
		class="fixed inset-x-3 bottom-3 z-50 rounded-2xl border border-[var(--surface-border)] bg-[var(--surface-bg)] p-4 shadow-2xl md:left-auto md:right-4 md:top-4 md:bottom-4 md:w-96"
	>
		<div class="mb-3 flex items-start justify-between gap-3">
			<div>
				<h3 class="text-sm font-semibold text-[var(--text-primary)]">
					Ask about: {scopeTitle}
				</h3>
				{#if scopeContext}
					<p class="mt-1 line-clamp-3 text-xs text-[var(--text-secondary)]">
						{scopeContext}
					</p>
				{/if}
			</div>
			<button
				type="button"
				class="rounded-md px-2 py-1 text-xs text-[var(--text-secondary)] hover:bg-[var(--surface-hover)]"
				onclick={onClose}
			>
				Close
			</button>
		</div>

		<div class="space-y-3">
			<input
				type="text"
				bind:value={question}
				placeholder="Ask a focused question..."
				class="w-full rounded-lg border border-[var(--surface-border)] bg-[var(--surface-hover)] px-3 py-2 text-sm text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none"
				onkeydown={(e) => {
					if (e.key === "Enter") submit();
				}}
			/>

			<div class="flex flex-wrap gap-2">
				{#each suggestions as suggestion}
					<button
						type="button"
						class="rounded-full border border-[var(--surface-border)] px-3 py-1 text-xs text-[var(--text-secondary)] hover:border-[var(--accent-primary)] hover:text-[var(--accent-primary)]"
						onclick={() => {
							question = suggestion;
						}}
					>
						{suggestion}
					</button>
				{/each}
			</div>

			<div class="rounded-lg border border-[var(--surface-border)] bg-[var(--surface-hover)] p-3 text-xs text-[var(--text-secondary)]">
				This opens Augur with the current Knowledge Home context attached.
			</div>

			<div class="flex justify-end">
				<button
					type="button"
					class="rounded-lg bg-[var(--interactive-text)] px-3 py-2 text-sm font-medium text-white"
					onclick={submit}
				>
					Ask in Augur
				</button>
			</div>
		</div>
	</div>
{/if}
