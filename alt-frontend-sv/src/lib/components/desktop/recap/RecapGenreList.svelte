<script lang="ts">
import type { RecapGenre } from "$lib/schema/recap";
import { cn } from "$lib/utils";
import {
	Sparkles,
	Briefcase,
	Clapperboard,
	Globe,
	Heart,
	Cpu,
	BookOpen,
	TrendingUp,
} from "@lucide/svelte";

interface Props {
	genres: RecapGenre[];
	selectedGenre: RecapGenre | null;
	onSelectGenre: (genre: RecapGenre) => void;
}

let { genres, selectedGenre, onSelectGenre }: Props = $props();

// Map genre names to icons
const genreIcons: Record<string, typeof Sparkles> = {
	AI: Cpu,
	Business: Briefcase,
	Entertainment: Clapperboard,
	Technology: Cpu,
	Health: Heart,
	Science: BookOpen,
	Politics: Globe,
	Sports: TrendingUp,
};

function getIcon(genreName: string) {
	return genreIcons[genreName] ?? Sparkles;
}

function handleSelect(genre: RecapGenre) {
	onSelectGenre(genre);
}
</script>

<div class="border border-[var(--surface-border)] bg-white">
	<div class="p-4 border-b border-[var(--surface-border)]">
		<h3 class="text-sm font-semibold text-[var(--text-primary)]">Genres</h3>
		<p class="text-xs text-[var(--text-secondary)] mt-1">{genres.length} topics</p>
	</div>

	<ul class="divide-y divide-[var(--surface-border)]">
		{#each genres as genre}
			{@const Icon = getIcon(genre.genre)}
			<li>
				<button
					type="button"
					onclick={() => handleSelect(genre)}
					class={cn(
						"w-full text-left px-4 py-3 transition-colors duration-200 hover:bg-[var(--surface-hover)]",
						selectedGenre?.genre === genre.genre &&
							"bg-[var(--surface-hover)] border-l-4 border-[var(--accent-primary)]"
					)}
				>
					<div class="flex items-start gap-3">
						<Icon
							class={cn(
								"h-4 w-4 mt-0.5 flex-shrink-0",
								selectedGenre?.genre === genre.genre
									? "text-[var(--accent-primary)]"
									: "text-[var(--text-secondary)]"
							)}
						/>
						<div class="flex-1 min-w-0">
							<h4
								class={cn(
									"text-sm font-medium truncate",
									selectedGenre?.genre === genre.genre
										? "text-[var(--accent-primary)]"
										: "text-[var(--text-primary)]"
								)}
							>
								{genre.genre}
							</h4>
							<p class="text-xs text-[var(--text-secondary)] mt-0.5">
								{genre.articleCount} articles · {genre.clusterCount} clusters
							</p>
						</div>
					</div>
				</button>
			</li>
		{/each}
	</ul>
</div>
