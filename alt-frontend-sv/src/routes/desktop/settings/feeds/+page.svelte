<script lang="ts">
	import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
	import { Button } from "$lib/components/ui/button";
	import { Plus } from "@lucide/svelte";

	// TODO: Implement feed links API
</script>

<PageHeader title="Manage Feed Links" description="Add, edit, or remove RSS feed sources" />

<div class="mb-6">
	<Button class="flex items-center gap-2">
		<Plus class="h-4 w-4" />
		Add New Feed
	</Button>
</div>

<div class="text-center py-12">
	<p class="text-[var(--text-secondary)] text-sm">
		Feed management feature coming soon
	</p>
</div>
