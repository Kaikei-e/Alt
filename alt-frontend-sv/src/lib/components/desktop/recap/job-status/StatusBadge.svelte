<script lang="ts">
	import type { JobStatus, GenreStatusType } from "$lib/schema/dashboard";

	interface Props {
		status: JobStatus | GenreStatusType;
		size?: "sm" | "md";
	}

	let { status, size = "md" }: Props = $props();

	const statusConfig: Record<
		JobStatus | GenreStatusType,
		{ bg: string; text: string; label: string }
	> = {
		pending: { bg: "bg-gray-100", text: "text-gray-700", label: "Pending" },
		running: { bg: "bg-blue-100", text: "text-blue-700", label: "Running" },
		completed: {
			bg: "bg-green-100",
			text: "text-green-700",
			label: "Completed",
		},
		succeeded: {
			bg: "bg-green-100",
			text: "text-green-700",
			label: "Succeeded",
		},
		failed: { bg: "bg-red-100", text: "text-red-700", label: "Failed" },
	};

	const config = $derived(
		statusConfig[status] ?? statusConfig.pending
	);
	const sizeClass = $derived(
		size === "sm" ? "px-2 py-0.5 text-xs" : "px-3 py-1 text-sm"
	);
</script>

<span
	class="inline-flex items-center font-medium rounded-full {config.bg} {config.text} {sizeClass}"
>
	{#if status === "running"}
		<span class="w-2 h-2 mr-1.5 bg-blue-500 rounded-full animate-pulse"></span>
	{/if}
	{config.label}
</span>
