<script lang="ts">
import { History } from "@lucide/svelte";
</script>

<div
	role="region"
	aria-label="Empty viewed feeds state"
	class="flex flex-col justify-center items-center min-h-[70vh] p-6 text-center"
>
	<div class="flex flex-col gap-8 max-w-[400px]">
		<!-- Icon -->
		<div
			data-testid="empty-viewed-feeds-icon"
			class="w-[120px] h-[120px] flex items-center justify-center rounded-full border-2 opacity-50"
			style="background: rgba(30, 30, 40, 0.4); border-color: rgba(255, 255, 255, 0.05);"
		>
			<History size={48} style="color: white;" />
		</div>

		<!-- Heading -->
		<div class="flex flex-col gap-3">
			<h2
				class="text-2xl md:text-3xl font-bold"
				style="color: white;"
			>
				No History Yet
			</h2>

			<p
				class="text-base leading-relaxed px-4"
				style="color: rgba(255, 255, 255, 0.6);"
			>
				Articles you read will appear here.
			</p>
		</div>
	</div>
</div>

