<script lang="ts">
import type { AcolyteReportSummary } from "$lib/connect/acolyte";

interface Props {
	report: AcolyteReportSummary;
	onClick: (reportId: string) => void;
}

const { report, onClick }: Props = $props();

const STATUS_COLORS: Record<string, string> = {
	succeeded: "var(--alt-sage, #7c9070)",
	running: "var(--alt-sand, #d4a574)",
	failed: "var(--alt-terracotta, #b85450)",
	pending: "var(--alt-ash, #999)",
};

const STATUS_LABELS: Record<string, string> = {
	succeeded: "Complete",
	running: "Running",
	failed: "Failed",
	pending: "Queued",
};

const status = $derived(report.latestRunStatus || "draft");
const stripeColor = $derived(
	STATUS_COLORS[status] ?? "var(--surface-border, #c8c8c8)",
);
const statusLabel = $derived(STATUS_LABELS[status] ?? "Draft");
const statusTextColor = $derived(
	STATUS_COLORS[status] ?? "var(--surface-border, #c8c8c8)",
);
const formattedType = $derived(report.reportType.replace(/_/g, " "));
const formattedDate = $derived(
	new Date(report.createdAt).toLocaleDateString("en-US", {
		month: "short",
		day: "numeric",
	}),
);
</script>

<div
	class="flex items-stretch border-b border-[var(--surface-border,#c8c8c8)] transition-colors duration-150 active:bg-[var(--surface-hover,#f3f1ed)]"
	data-testid="report-card-{report.reportId}"
	role="button"
	tabindex="0"
	onclick={() => onClick(report.reportId)}
	onkeydown={(e) => { if (e.key === "Enter") onClick(report.reportId); }}
>
	<!-- Status stripe -->
	<div
		class="w-[3px] shrink-0"
		style="background: {stripeColor};"
		data-testid="status-stripe-{report.reportId}"
		data-status={status}
	></div>

	<!-- Card body -->
	<div class="flex-1 min-w-0 px-4 py-3">
		<div class="flex items-baseline justify-between mb-0.5">
			<span
				class="font-[var(--font-body)] text-[0.65rem] uppercase tracking-wider text-[var(--alt-ash,#999)]"
			>
				{formattedType}
			</span>
			<span
				class="font-[var(--font-mono)] text-[0.65rem] font-semibold text-[var(--alt-slate,#666)] border border-[var(--surface-border,#c8c8c8)] px-1.5 leading-relaxed"
			>
				v{report.currentVersion}
			</span>
		</div>
		<h2
			class="font-[var(--font-display)] text-base font-bold leading-snug text-[var(--alt-charcoal,#1a1a1a)] truncate"
		>
			{report.title}
		</h2>
		<div class="flex items-center justify-between mt-1">
			<span class="font-[var(--font-body)] text-[0.7rem] text-[var(--alt-ash,#999)]">
				{formattedDate}
			</span>
			<span
				class="font-[var(--font-body)] text-[0.65rem] font-semibold uppercase tracking-wide"
				style="color: {statusTextColor};"
			>
				{statusLabel}
			</span>
		</div>
	</div>

</div>
