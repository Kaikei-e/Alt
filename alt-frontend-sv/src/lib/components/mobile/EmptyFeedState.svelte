<script lang="ts">
	import { Plus, Rss } from "@lucide/svelte";
	import { Button } from "$lib/components/ui/button";
</script>

<div
	role="region"
	aria-label="Empty feed state"
	class="flex flex-col justify-center items-center min-h-[70vh] p-6 text-center"
>
	<div class="flex flex-col gap-8 max-w-[400px]">
		<!-- Animated Icon -->
		<div
			data-testid="empty-state-icon"
			class="w-[120px] h-[120px] flex items-center justify-center rounded-full border-2"
			style="background: var(--alt-glass); border-color: var(--alt-glass-border);"
		>
			<Rss
				size={56}
				style="color: var(--alt-text-secondary);"
				stroke-width="1.5"
			/>
		</div>

		<!-- Heading -->
		<div class="flex flex-col gap-3">
			<h2
				class="text-2xl md:text-3xl font-bold"
				style="color: var(--alt-text-primary); background: var(--accent-gradient); -webkit-background-clip: text; -webkit-text-fill-color: transparent; background-clip: text;"
			>
				No Feeds Yet
			</h2>

			<p
				class="text-base leading-relaxed px-4"
				style="color: var(--alt-text-secondary);"
			>
				Start by adding your first RSS feed to begin reading articles
				from your favorite sources.
			</p>
		</div>

		<!-- Call to Action Button -->
		<a href="/sv/mobile/feeds/manage">
			<Button
				size="lg"
				class="rounded-2xl font-bold px-8"
				style="background: linear-gradient(to right, #FF416C, #FF4B2B); color: white;"
			>
				<div class="flex items-center gap-2">
					<Plus size={20} />
					<span>Add Your First Feed</span>
				</div>
			</Button>
		</a>
	</div>
</div>
