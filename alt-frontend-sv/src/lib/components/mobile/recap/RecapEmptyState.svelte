<script lang="ts">
import { Calendar } from "@lucide/svelte";
</script>

<div
	role="region"
	aria-label="Empty recap state"
	class="flex flex-col justify-center items-center min-h-[70vh] p-6 text-center"
>
	<div class="flex flex-col gap-8 max-w-[400px]">
		<div
			class="w-[120px] h-[120px] flex items-center justify-center rounded-full border-2 mx-auto"
			style="background: var(--alt-glass); border-color: var(--alt-glass-border);"
		>
			<Calendar size={56} style="color: var(--alt-text-secondary);" stroke-width={1.5} />
		</div>

		<div class="flex flex-col gap-3">
			<h2 class="text-2xl font-bold" style="color: var(--alt-text-secondary);">
				No Recap Yet
			</h2>
			<p class="text-base leading-relaxed px-4" style="color: var(--alt-text-secondary);">
				Your recap summary is being prepared. Check back soon!
			</p>
		</div>
	</div>
</div>
