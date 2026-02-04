<script lang="ts">
import DesktopTagTrailScreen from "$lib/components/desktop/tag-trail/DesktopTagTrailScreen.svelte";

const { data } = $props();
</script>

<DesktopTagTrailScreen initialFeed={data.initialFeed} />
