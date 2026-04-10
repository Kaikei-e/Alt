<script lang="ts">
type Props = {
	onSend: (message: string) => void;
	disabled?: boolean;
	initialContext?: string;
};

let { onSend, disabled = false, initialContext = "" }: Props = $props();

let inputValue = $state("");

$effect(() => {
	inputValue = initialContext;
});

function handleSubmit() {
	const trimmed = inputValue.trim();
	if (!trimmed || disabled) return;

	onSend(trimmed);
	inputValue = "";
}

function handleKeydown(event: KeyboardEvent) {
	// Ignore Enter during IME composition (e.g., Japanese input)
	if (event.isComposing) return;

	// Send on Enter (without Shift)
	if (event.key === "Enter" && !event.shiftKey) {
		event.preventDefault();
		handleSubmit();
	}
}
</script>

<div class="augur-input-area">
	<div class="input-row">
		<textarea
			id="augur-input"
			class="input-field"
			bind:value={inputValue}
			onkeydown={handleKeydown}
			placeholder="What would you like to know?"
			{disabled}
			rows={2}
		></textarea>
		<button
			class="input-submit"
			onclick={handleSubmit}
			disabled={disabled || !inputValue.trim()}
			aria-label="Submit question"
		>
			Submit
		</button>
	</div>
	<p class="input-hint">Press Enter to submit, Shift+Enter for new line</p>
</div>

<style>
	.augur-input-area {
		padding: 0 0 1rem;
		background: var(--surface-bg, #faf9f7);
	}
	.input-row {
		display: flex; gap: 0.5rem; align-items: flex-end;
	}
	.input-field {
		flex: 1;
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.95rem; line-height: 1.5;
		padding: 0.6rem 0.75rem;
		border: 1px solid var(--surface-border, #c8c8c8);
		border-radius: 0;
		background: transparent;
		color: var(--alt-charcoal, #1a1a1a);
		resize: none;
		min-height: 44px; max-height: 120px;
		transition: border-color 0.15s;
	}
	.input-field::placeholder {
		color: var(--alt-ash, #999);
		font-style: italic;
	}
	.input-field:focus {
		outline: none;
		border-color: var(--alt-charcoal, #1a1a1a);
	}
	.input-field:disabled {
		opacity: 0.5; cursor: not-allowed;
	}
	.input-submit {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.75rem; font-weight: 600;
		letter-spacing: 0.06em; text-transform: uppercase;
		padding: 0.55rem 1.1rem;
		border: 1.5px solid var(--alt-charcoal, #1a1a1a);
		background: transparent;
		color: var(--alt-charcoal, #1a1a1a);
		cursor: pointer;
		transition: background-color 0.2s, color 0.2s;
		min-height: 44px;
		white-space: nowrap;
	}
	.input-submit:hover:not(:disabled) {
		background: var(--alt-charcoal, #1a1a1a);
		color: var(--surface-bg, #faf9f7);
	}
	.input-submit:disabled {
		opacity: 0.4; cursor: not-allowed;
	}
	.input-hint {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.7rem; font-style: italic;
		color: var(--alt-ash, #999);
		margin: 0.4rem 0 0;
	}
</style>
