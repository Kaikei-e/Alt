<script lang="ts">
let {
	values,
	width = 120,
	height = 32,
	threshold,
	stroke = "var(--obs-spark-stroke, var(--alt-primary))",
	thresholdStroke = "var(--obs-spark-threshold, var(--alt-warning))",
}: {
	values: number[];
	width?: number;
	height?: number;
	/** Optional value at which to draw a dashed reference line. */
	threshold?: number;
	stroke?: string;
	thresholdStroke?: string;
} = $props();

const padding = 2;

const domain = $derived(() => {
	if (values.length < 2) return null;
	const valueMin = Math.min(...values);
	const valueMax = Math.max(...values);
	const min =
		threshold !== undefined ? Math.min(valueMin, threshold) : valueMin;
	const max =
		threshold !== undefined ? Math.max(valueMax, threshold) : valueMax;
	return { min, max, range: max - min || 1 };
});

const points = $derived(() => {
	const d = domain();
	if (!d) return "";
	const stepX = (width - padding * 2) / (values.length - 1);
	return values
		.map((v, i) => {
			const x = padding + i * stepX;
			const y =
				height - padding - ((v - d.min) / d.range) * (height - padding * 2);
			return `${x},${y}`;
		})
		.join(" ");
});

const lastPoint = $derived(() => {
	const d = domain();
	if (!d) return null;
	const stepX = (width - padding * 2) / (values.length - 1);
	const lastIdx = values.length - 1;
	const x = padding + lastIdx * stepX;
	const y =
		height -
		padding -
		((values[lastIdx] - d.min) / d.range) * (height - padding * 2);
	return { x, y };
});

const thresholdY = $derived(() => {
	const d = domain();
	if (!d || threshold === undefined) return null;
	return height - padding - ((threshold - d.min) / d.range) * (height - padding * 2);
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
		{#if thresholdY() !== null}
			<line
				x1="0"
				x2={width}
				y1={thresholdY()}
				y2={thresholdY()}
				stroke={thresholdStroke}
				stroke-width="1"
				stroke-dasharray="3 3"
				opacity="0.6"
				data-testid="sparkline-threshold"
			/>
		{/if}
		<polyline
			points={points()}
			fill="none"
			{stroke}
			stroke-width="1.25"
			stroke-linecap="round"
			stroke-linejoin="round"
		/>
		{#if lastPoint()}
			<circle
				cx={lastPoint()!.x}
				cy={lastPoint()!.y}
				r="2"
				fill={stroke}
			/>
		{/if}
	</svg>
{/if}
