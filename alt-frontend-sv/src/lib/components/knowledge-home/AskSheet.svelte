<script lang="ts">
import { FileText } from "@lucide/svelte";

interface Props {
	open: boolean;
	scopeTitle: string;
	scopeContext?: string;
	scopeArticleId?: string;
	scopeTags?: string[];
	onClose: () => void;
	onSubmit: (question: string) => void;
}

const { open, scopeTitle, scopeContext, scopeArticleId, scopeTags, onClose, onSubmit }: Props = $props();

let question = $state("");

const suggestions = [
	"この記事の要点は？",
	"ここでの新しい発見は？",
	"次に何を読むべき？",
] as const;

function submit() {
	const trimmed = question.trim();
	if (!trimmed) return;
	onSubmit(trimmed);
}
</script>

{#if open}
	<div class="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm" role="presentation" onclick={onClose}></div>
	<div
		class="fixed inset-x-3 bottom-3 z-50 rounded-2xl border border-[var(--surface-border)] bg-[var(--surface-bg)] p-4 shadow-2xl animate-slide-in-bottom md:animate-slide-in-right md:left-auto md:right-4 md:top-4 md:bottom-4 md:w-96"
	>
		<div class="mb-3 flex items-start justify-between gap-3">
			<div class="min-w-0 flex-1">
				{#if scopeArticleId}
					<p class="text-xs font-medium text-[var(--text-secondary)]">質問の対象:</p>
					<div class="mt-2 flex items-start gap-2.5 rounded-lg border border-[var(--surface-border)] bg-[var(--surface-hover)] p-2.5">
						<FileText class="mt-0.5 h-4 w-4 flex-shrink-0 text-[var(--interactive-text)]" />
						<div class="min-w-0 flex-1">
							<p class="line-clamp-2 text-sm font-medium text-[var(--text-primary)]">{scopeTitle}</p>
							{#if scopeTags && scopeTags.length > 0}
								<div class="mt-1.5 flex flex-wrap gap-1">
									{#each scopeTags as tag}
										<span class="rounded-full bg-[var(--surface-bg)] px-2 py-0.5 text-[10px] text-[var(--text-secondary)]">
											{tag}
										</span>
									{/each}
								</div>
							{/if}
						</div>
					</div>
				{:else}
					<h3 class="text-sm font-semibold text-[var(--text-primary)]">
						{scopeTitle} について質問
					</h3>
					{#if scopeContext}
						<p class="mt-1 text-xs text-[var(--text-secondary)]">
							{scopeContext}
						</p>
					{/if}
				{/if}
			</div>
			<button
				type="button"
				class="rounded-md px-2 py-1 text-xs text-[var(--text-secondary)] hover:bg-[var(--surface-hover)]"
				onclick={onClose}
			>
				閉じる
			</button>
		</div>

		<div class="space-y-3">
			<input
				type="text"
				bind:value={question}
				placeholder="質問を入力..."
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
				現在の Knowledge Home のコンテキストを添えて Augur に質問します。
			</div>

			<div class="flex justify-end">
				<button
					type="button"
					class="rounded-lg bg-[var(--interactive-text)] px-3 py-2 text-sm font-medium text-white"
					onclick={submit}
				>
					Augur に質問
				</button>
			</div>
		</div>
	</div>
{/if}
