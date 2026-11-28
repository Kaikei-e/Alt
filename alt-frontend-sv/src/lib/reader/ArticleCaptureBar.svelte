<script lang="ts">
import {
	type AnnotationStance,
	type ArticleAnnotationItem,
	deleteArticleAnnotation,
	listArticleAnnotations,
	saveArticleAnnotation,
} from "#lib/connect/reading.js";
import { getReadingTransport } from "#lib/connect/reading-transport.js";
import {
	ANNOTATION_STANCE_OPTIONS,
	buildArticleAugurHref,
	clampMicroText,
	MAX_CURATION_TEXT_LENGTH,
	stanceLabel,
} from "#lib/reading/curation-helpers.js";
import { isUuid } from "#lib/reading/reading-events.js";

interface Props {
	articleId: string | null;
	articleTitle?: string | null;
}

const { articleId, articleTitle = null }: Props = $props();

const validArticleId = $derived(isUuid(articleId) ? articleId : null);

let annotations = $state<ArticleAnnotationItem[]>([]);
let activeStance = $state<AnnotationStance | null>(null);
let excerpt = $state("");
let note = $state("");
let isSaving = $state(false);
let errorMessage = $state<string | null>(null);
let loadSeq = 0;

const activeOption = $derived(
	ANNOTATION_STANCE_OPTIONS.find((opt) => opt.stance === activeStance) ?? null,
);

const canSave = $derived(
	Boolean(
		validArticleId &&
			activeStance !== null &&
			!isSaving &&
			(excerpt.trim().length > 0 || note.trim().length > 0),
	),
);

$effect(() => {
	const id = validArticleId;
	const seq = ++loadSeq;
	activeStance = null;
	excerpt = "";
	note = "";
	errorMessage = null;

	if (!id) {
		annotations = [];
		return;
	}

	void Promise.resolve(listArticleAnnotations(getReadingTransport(), [id], 20))
		.then((list) => {
			if (seq !== loadSeq) return;
			annotations = Array.isArray(list) ? list : [];
		})
		.catch(() => {
			if (seq !== loadSeq) return;
			annotations = [];
		});
});

function readWindowSelection(): string {
	if (typeof window === "undefined") return "";
	const raw = window.getSelection?.()?.toString() ?? "";
	return clampMicroText(raw, MAX_CURATION_TEXT_LENGTH);
}

function selectStance(stance: AnnotationStance) {
	errorMessage = null;
	if (activeStance === stance && !excerpt.trim() && !note.trim()) {
		activeStance = null;
		return;
	}
	activeStance = stance;
	if (!excerpt.trim()) {
		const selected = readWindowSelection();
		if (selected) {
			excerpt = selected;
		}
	}
}

function handleUseSelection() {
	const selected = readWindowSelection();
	if (selected) {
		excerpt = selected;
	}
}

function handleCancel() {
	activeStance = null;
	excerpt = "";
	note = "";
	errorMessage = null;
}

async function handleSave() {
	if (!validArticleId || activeStance === null || isSaving) return;
	const cleanedExcerpt = clampMicroText(excerpt, MAX_CURATION_TEXT_LENGTH);
	const cleanedNote = clampMicroText(note, MAX_CURATION_TEXT_LENGTH);
	if (!cleanedExcerpt && !cleanedNote) {
		errorMessage = "Add a quote or a short note before saving.";
		return;
	}

	isSaving = true;
	errorMessage = null;
	try {
		const saved = await saveArticleAnnotation(getReadingTransport(), {
			articleId: validArticleId,
			stance: activeStance,
			excerpt: cleanedExcerpt,
			note: cleanedNote,
		});
		annotations = [saved, ...annotations];
		activeStance = null;
		excerpt = "";
		note = "";
	} catch {
		errorMessage = "Could not save takeaway. Please try again.";
	} finally {
		isSaving = false;
	}
}

async function handleDelete(annotationId: string) {
	errorMessage = null;
	try {
		await deleteArticleAnnotation(getReadingTransport(), annotationId);
		annotations = annotations.filter((a) => a.annotationId !== annotationId);
	} catch {
		errorMessage = "Could not remove takeaway. Please try again.";
	}
}

function augurHrefFor(ann: ArticleAnnotationItem): string {
	const question = ann.note.trim() || ann.excerpt.trim();
	return buildArticleAugurHref({
		question,
		articleTitle,
		articleId: ann.articleId,
	});
}
</script>

{#if validArticleId}
	<section
		class="capture-bar"
		aria-label="Article takeaways"
		data-testid="article-capture-bar"
	>
		<div class="capture-header">
			<span class="capture-kicker">Takeaway</span>
			<div class="stance-row" role="group" aria-label="Capture stance">
				{#each ANNOTATION_STANCE_OPTIONS as opt (opt.stance)}
					<button
						type="button"
						class="stance-pill"
						class:stance-pill--active={activeStance === opt.stance}
						aria-pressed={activeStance === opt.stance}
						onclick={() => selectStance(opt.stance)}
					>
						{opt.label}
					</button>
				{/each}
			</div>
		</div>

		{#if activeOption}
			<form
				class="capture-composer"
				data-testid="article-capture-composer"
				onsubmit={(e) => {
					e.preventDefault();
					void handleSave();
				}}
			>
				<div class="composer-field">
					<div class="composer-field-head">
						<label for="capture-excerpt" class="field-label">Quote (optional)</label>
						<button
							type="button"
							class="quiet-action"
							onclick={handleUseSelection}
						>
							Use selection
						</button>
					</div>
					<textarea
						id="capture-excerpt"
						class="field-input"
						rows={2}
						maxlength={MAX_CURATION_TEXT_LENGTH}
						placeholder="Optional quote from the article"
						bind:value={excerpt}
					></textarea>
				</div>

				<div class="composer-field">
					<label for="capture-note" class="field-label">1-line note</label>
					<input
						id="capture-note"
						type="text"
						class="field-input"
						maxlength={MAX_CURATION_TEXT_LENGTH}
						placeholder={activeOption.placeholder}
						bind:value={note}
					/>
				</div>

				<div class="composer-actions">
					<button
						type="submit"
						class="save-btn"
						disabled={!canSave}
					>
						{isSaving ? "Saving…" : "Save takeaway"}
					</button>
					<button
						type="button"
						class="quiet-action"
						onclick={handleCancel}
					>
						Cancel
					</button>
				</div>
			</form>
		{/if}

		{#if errorMessage}
			<p class="capture-error" role="alert" data-testid="article-capture-error">
				{errorMessage}
			</p>
		{/if}

		{#if annotations.length > 0}
			<ul class="takeaway-list" data-testid="article-takeaway-list">
				{#each annotations as ann (ann.annotationId)}
					<li class="takeaway-item">
						<div class="takeaway-head">
							<span class="takeaway-stance">{stanceLabel(ann.stance)}</span>
							<div class="takeaway-actions">
								<a href={augurHrefFor(ann)} class="quiet-action">Ask Augur</a>
								<button
									type="button"
									class="quiet-action"
									aria-label="Remove takeaway"
									onclick={() => void handleDelete(ann.annotationId)}
								>
									Remove
								</button>
							</div>
						</div>
						{#if ann.excerpt}
							<blockquote class="takeaway-excerpt">{ann.excerpt}</blockquote>
						{/if}
						{#if ann.note}
							<p class="takeaway-note">{ann.note}</p>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</section>
{/if}

<style>
	.capture-bar {
		margin-top: 1.5rem;
		padding-top: 1rem;
		border-top: 1px solid var(--surface-border, #c8c8c8);
		font-family: var(--font-body);
		color: var(--alt-charcoal, #1a1a1a);
	}

	.capture-header {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
	}

	.capture-kicker {
		font-family: var(--font-mono);
		font-size: 0.72rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-slate, #666666);
	}

	.stance-row {
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem;
	}

	.stance-pill {
		min-height: 44px;
		padding: 0 0.75rem;
		border: 1px solid var(--surface-border, #c8c8c8);
		background: transparent;
		font-family: var(--font-mono);
		font-size: 0.72rem;
		color: var(--alt-charcoal, #1a1a1a);
		cursor: pointer;
		touch-action: manipulation;
	}

	.stance-pill--active {
		border-color: var(--alt-charcoal, #1a1a1a);
		background: var(--alt-charcoal, #1a1a1a);
		color: var(--surface-bg, #faf9f7);
	}

	.capture-composer {
		margin-top: 0.75rem;
		padding: 0.75rem;
		border: 1px solid var(--surface-border, #c8c8c8);
		background: var(--surface-2, #f5f4f1);
		display: flex;
		flex-direction: column;
		gap: 0.65rem;
	}

	.composer-field {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.composer-field-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
	}

	.field-label {
		font-family: var(--font-mono);
		font-size: 0.68rem;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--alt-slate, #666666);
	}

	.field-input {
		width: 100%;
		box-sizing: border-box;
		padding: 0.5rem 0.6rem;
		border: 1px solid var(--surface-border, #c8c8c8);
		background: var(--surface-bg, #faf9f7);
		font-family: var(--font-body);
		font-size: 0.95rem;
		line-height: 1.45;
		color: var(--alt-charcoal, #1a1a1a);
	}

	.field-input:focus {
		outline: none;
		border-color: var(--alt-charcoal, #1a1a1a);
	}

	.composer-actions {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}

	.save-btn {
		min-height: 44px;
		padding: 0 1rem;
		border: 1.5px solid var(--alt-charcoal, #1a1a1a);
		background: var(--alt-charcoal, #1a1a1a);
		color: var(--surface-bg, #faf9f7);
		font-family: var(--font-body);
		font-size: 0.82rem;
		font-weight: 600;
		cursor: pointer;
	}

	.save-btn:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	.quiet-action {
		display: inline-flex;
		align-items: center;
		min-height: 44px;
		padding: 0 0.2rem;
		border: none;
		background: transparent;
		font-family: var(--font-mono);
		font-size: 0.72rem;
		color: var(--alt-slate, #666666);
		text-decoration: underline;
		text-underline-offset: 0.2em;
		cursor: pointer;
		touch-action: manipulation;
	}

	.capture-error {
		margin: 0.5rem 0 0;
		padding: 0.4rem 0.65rem;
		border-left: 3px solid var(--alt-error, #8c1d1d);
		background: var(--surface-2, #f5f4f1);
		font-size: 0.82rem;
		color: var(--alt-error, #8c1d1d);
	}

	.takeaway-list {
		list-style: none;
		margin: 0.75rem 0 0;
		padding: 0;
	}

	.takeaway-item {
		padding: 0.65rem 0;
		border-top: 1px solid var(--surface-border, #c8c8c8);
	}

	.takeaway-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
	}

	.takeaway-stance {
		font-family: var(--font-mono);
		font-size: 0.68rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--alt-primary, #2f4f4f);
	}

	.takeaway-actions {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}

	.takeaway-excerpt {
		margin: 0.25rem 0 0;
		padding-left: 0.65rem;
		border-left: 2px solid var(--surface-border, #c8c8c8);
		font-size: 0.88rem;
		line-height: 1.5;
		color: var(--alt-slate, #666666);
		overflow-wrap: anywhere;
	}

	.takeaway-note {
		margin: 0.35rem 0 0;
		font-size: 0.94rem;
		line-height: 1.5;
		color: var(--alt-charcoal, #1a1a1a);
		overflow-wrap: anywhere;
	}
</style>
