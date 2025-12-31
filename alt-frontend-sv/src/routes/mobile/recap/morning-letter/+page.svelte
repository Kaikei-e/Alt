<script lang="ts">
	import MorningLetterChat from "$lib/components/mobile/morning-letter/MorningLetterChat.svelte";
	import FloatingMenu from "$lib/components/mobile/feeds/swipe/FloatingMenu.svelte";
</script>

<div class="min-h-[100dvh] flex flex-col" style="background: var(--app-bg);">
	<div class="flex-1">
		<MorningLetterChat withinHours={24} />
	</div>
	<FloatingMenu />
</div>
