<script lang="ts">
	import { Send } from "@lucide/svelte";
	import { Button } from "$lib/components/ui/button";
	import { Textarea } from "$lib/components/ui/textarea";

	type Props = {
		onSend: (message: string) => void;
		disabled?: boolean;
	};

	let { onSend, disabled = false }: Props = $props();

	let inputValue = $state("");

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

<div class="border-t border-border bg-background p-4">
	<div class="flex gap-2">
		<Textarea
			bind:value={inputValue}
			onkeydown={handleKeydown}
			placeholder="Ask Augur anything..."
			class="flex-1 resize-none min-h-[44px] max-h-[120px] rounded-full border-border/50 bg-muted/30"
			{disabled}
			rows={1}
		/>
		<Button
			onclick={handleSubmit}
			{disabled}
			class="flex-shrink-0 px-4"
			aria-label="Send message"
		>
			<Send class="h-4 w-4" />
		</Button>
	</div>
	<p class="text-xs text-muted-foreground mt-2">
		Press Enter to send, Shift+Enter for new line
	</p>
</div>
