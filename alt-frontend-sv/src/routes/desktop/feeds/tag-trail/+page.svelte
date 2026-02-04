<script lang="ts">
import DesktopLayout from "$lib/components/desktop/layout/DesktopLayout.svelte";
import DesktopTagTrailScreen from "$lib/components/desktop/tag-trail/DesktopTagTrailScreen.svelte";

const { data } = $props();
</script>

<DesktopLayout class="py-6 pr-6">
	<DesktopTagTrailScreen initialFeed={data.initialFeed} />
</DesktopLayout>
