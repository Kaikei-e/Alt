<script lang="ts">
type Props = {
	minHeight?: string;
};

let { minHeight = "60vh" }: Props = $props();
</script>

<div class="morning-letter-skeleton animate-pulse space-y-6 p-4" style="min-height: {minHeight};">
	<!-- Date line -->
	<div class="h-4 w-32 rounded" style="background: var(--skeleton-base, #e8e4de);"></div>

	<!-- Lead paragraph -->
	<div class="space-y-2">
		<div class="h-6 w-full rounded" style="background: var(--skeleton-base, #e8e4de);"></div>
		<div class="h-6 w-4/5 rounded" style="background: var(--skeleton-base, #e8e4de);"></div>
	</div>

	<!-- Section 1 -->
	<div class="border-t-2 pt-4" style="border-color: var(--skeleton-base, #e8e4de);">
		<div class="h-5 w-40 rounded mb-3" style="background: var(--skeleton-base, #e8e4de);"></div>
		<div class="space-y-2">
			<div class="h-4 w-full rounded" style="background: var(--skeleton-base, #e8e4de);"></div>
			<div class="h-4 w-11/12 rounded" style="background: var(--skeleton-base, #e8e4de);"></div>
			<div class="h-4 w-3/4 rounded" style="background: var(--skeleton-base, #e8e4de);"></div>
		</div>
	</div>

	<!-- Section 2 -->
	<div class="border-t-2 pt-4" style="border-color: var(--skeleton-base, #e8e4de);">
		<div class="h-5 w-36 rounded mb-3" style="background: var(--skeleton-base, #e8e4de);"></div>
		<div class="space-y-2">
			<div class="h-4 w-full rounded" style="background: var(--skeleton-base, #e8e4de);"></div>
			<div class="h-4 w-5/6 rounded" style="background: var(--skeleton-base, #e8e4de);"></div>
		</div>
	</div>
</div>
