<script lang="ts">
import type { TopicClusterSummary } from "./scene/sceneModel.js";

interface Props {
	clusters: readonly TopicClusterSummary[];
	focusedClusterId?: number | null;
	onFocusCluster: (clusterId: number | null) => void;
}

let {
	clusters: _clusters,
	focusedClusterId: _focusedClusterId = null,
	onFocusCluster: _onFocusCluster,
}: Props = $props();
</script>
