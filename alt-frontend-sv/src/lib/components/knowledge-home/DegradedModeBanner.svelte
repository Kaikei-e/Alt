<script lang="ts">
import { AlertTriangle } from "@lucide/svelte";
</script>

<div
	class="flex items-center gap-2 rounded-lg border px-4 py-2 text-sm"
	style="background: var(--warning-bg, #fef3cd); border-color: var(--warning-border, #ffc107); color: var(--warning-text, #856404);"
>
	<AlertTriangle size={16} />
	<span>Some data sources are temporarily unavailable. Showing partial results.</span>
</div>
