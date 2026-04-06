<script lang="ts">
let {
	values,
	width = 120,
	height = 32,
}: {
	values: number[];
	width?: number;
	height?: number;
} = $props();

const padding = 2;

const points = $derived(() => {
	if (values.length < 2) return "";
	const min = Math.min(...values);
	const max = Math.max(...values);
	const range = max - min || 1;
	const stepX = (width - padding * 2) / (values.length - 1);

	return values
		.map((v, i) => {
			const x = padding + i * stepX;
			const y = height - padding - ((v - min) / range) * (height - padding * 2);
			return `${x},${y}`;
		})
		.join(" ");
});

const lastPoint = $derived(() => {
	if (values.length < 2) return null;
	const min = Math.min(...values);
	const max = Math.max(...values);
	const range = max - min || 1;
	const stepX = (width - padding * 2) / (values.length - 1);
	const lastIdx = values.length - 1;
	const x = padding + lastIdx * stepX;
	const y =
		height - padding - ((values[lastIdx] - min) / range) * (height - padding * 2);
	return { x, y };
});
</script>

{#if values.length >= 2}
	<svg
		{width}
		{height}
		viewBox="0 0 {width} {height}"
		class="inline-block"
		style="overflow: visible;"
	>
		<polyline
			points={points()}
			fill="none"
			stroke="var(--accent-blue, #3b82f6)"
			stroke-width="1.5"
			stroke-linecap="round"
			stroke-linejoin="round"
		/>
		{#if lastPoint()}
			<circle
				cx={lastPoint()!.x}
				cy={lastPoint()!.y}
				r="2.5"
				fill="var(--accent-blue, #3b82f6)"
			/>
		{/if}
	</svg>
{/if}
