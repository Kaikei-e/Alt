<script lang="ts">
import {
	Home,
	Rss,
	Eye,
	Star,
	Search,
	CalendarRange,
	Newspaper,
	BirdIcon,
	ChartBar,
	Settings,
	LinkIcon,
	Plus,
	ChevronDown,
	Activity,
	Sparkles,
	Shuffle,
	Compass,
} from "@lucide/svelte";
import { page } from "$app/state";
import { cn } from "$lib/utils";

const svBasePath = "/sv";

const menuItems = [
	{
		label: "Dashboard",
		href: `${svBasePath}/desktop`,
		icon: Home,
		category: "main",
	},
	{
		label: "Feeds",
		category: "feeds",
		icon: Rss,
		children: [
			{
				label: "Unread Feeds",
				href: `${svBasePath}/desktop/feeds`,
				icon: Rss,
			},
			{
				label: "Read History",
				href: `${svBasePath}/desktop/feeds/viewed`,
				icon: Eye,
			},
			{
				label: "Favorites",
				href: `${svBasePath}/desktop/feeds/favorites`,
				icon: Star,
			},
			{
				label: "Search",
				href: `${svBasePath}/desktop/feeds/search`,
				icon: Search,
			},
		],
	},
	{
		label: "Recap",
		category: "recap",
		icon: CalendarRange,
		children: [
			{
				label: "3-Day Summary",
				href: `${svBasePath}/desktop/recap`,
				icon: CalendarRange,
			},
			{
				label: "Morning Letter",
				href: `${svBasePath}/desktop/recap/morning-letter`,
				icon: Newspaper,
			},
			{
				label: "Evening Pulse",
				href: `${svBasePath}/desktop/recap/evening-pulse`,
				icon: Sparkles,
			},
			{
				label: "Job Status",
				href: `${svBasePath}/desktop/recap/job-status`,
				icon: Activity,
			},
		],
	},
	{
		label: "Explore",
		category: "explore",
		icon: Compass,
		children: [
			{
				label: "Tag Trail",
				href: `${svBasePath}/desktop/feeds/tag-trail`,
				icon: Shuffle,
			},
		],
	},
	{
		label: "Ask Augur",
		href: `${svBasePath}/desktop/augur`,
		icon: BirdIcon,
		category: "main",
	},
	{
		label: "Settings",
		category: "settings",
		icon: Settings,
		children: [
			{
				label: "Manage Feed Links",
				href: `${svBasePath}/desktop/settings/feeds`,
				icon: LinkIcon,
			},
		],
	},
	{
		label: "Statistics",
		href: `${svBasePath}/desktop/stats`,
		icon: ChartBar,
		category: "main",
	},
];

let expandedSections = $state<string[]>([
	"feeds",
	"explore",
	"recap",
	"settings",
]);

function toggleSection(category: string) {
	if (expandedSections.includes(category)) {
		expandedSections = expandedSections.filter((c) => c !== category);
	} else {
		expandedSections = [...expandedSections, category];
	}
}

function isActive(href: string): boolean {
	return page.url.pathname === href;
}

function isParentActive(children?: { href: string }[]): boolean {
	if (!children) return false;
	return children.some((child) => page.url.pathname === child.href);
}
</script>

<aside
	class="sticky top-0 self-start h-screen w-60 flex-shrink-0 border-r border-[var(--surface-border)] bg-[var(--surface-bg)] overflow-y-auto"
>
	<!-- Logo/Brand -->
	<div class="p-6 border-b border-[var(--surface-border)]">
		<h2 class="text-xl font-bold text-[var(--text-primary)]">Alt Reader</h2>
		<p class="text-xs text-[var(--text-secondary)] mt-1">Desktop</p>
	</div>

	<!-- Navigation -->
	<nav class="p-4">
		<ul class="space-y-1">
			{#each menuItems as item}
				{#if item.children}
					<!-- Section with children -->
					<li>
						<button
							type="button"
							onclick={() => toggleSection(item.category)}
							class={cn(
								"w-full flex items-center justify-between px-3 py-2 text-sm font-medium transition-colors duration-200",
								"hover:bg-[var(--surface-hover)]",
								isParentActive(item.children)
									? "text-[var(--accent-primary)]"
									: "text-[var(--text-primary)]",
							)}
						>
							<div class="flex items-center gap-2">
								<item.icon class="h-4 w-4" />
								<span>{item.label}</span>
							</div>
							<ChevronDown
								class={cn(
									"h-4 w-4 transition-transform duration-200",
									expandedSections.includes(item.category) ? "rotate-180" : "",
								)}
							/>
						</button>
						{#if expandedSections.includes(item.category)}
							<ul class="ml-6 mt-1 space-y-1">
								{#each item.children as child}
									<li>
										<a
											href={child.href}
											class={cn(
												"flex items-center gap-2 px-3 py-2 text-sm transition-colors duration-200",
												isActive(child.href)
													? "bg-[var(--surface-hover)] text-[var(--accent-primary)] font-medium"
													: "text-[var(--text-secondary)] hover:bg-[var(--surface-hover)] hover:text-[var(--text-primary)]",
											)}
										>
											<child.icon class="h-3.5 w-3.5" />
											<span>{child.label}</span>
										</a>
									</li>
								{/each}
							</ul>
						{/if}
					</li>
				{:else}
					<!-- Simple link -->
					<li>
						<a
							href={item.href}
							class={cn(
								"flex items-center gap-2 px-3 py-2 text-sm font-medium transition-colors duration-200",
								isActive(item.href)
									? "bg-[var(--surface-hover)] text-[var(--accent-primary)]"
									: "text-[var(--text-primary)] hover:bg-[var(--surface-hover)]",
							)}
						>
							<item.icon class="h-4 w-4" />
							<span>{item.label}</span>
						</a>
					</li>
				{/if}
			{/each}
		</ul>
	</nav>
</aside>
