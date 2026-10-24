<script lang="ts">
import {
	Activity,
	Bell,
	BirdIcon,
	CalendarRange,
	ChartBar,
	ChevronDown,
	Compass,
	Eye,
	Footprints,
	GalleryVerticalEnd,
	Gauge,
	Home,
	Lightbulb,
	LinkIcon,
	MessagesSquare,
	Newspaper,
	Orbit,
	Rss,
	ScrollText,
	Search,
	Settings,
	Shuffle,
	Sparkles,
	Star,
	Tag,
} from "@lucide/svelte";
import { cn } from "#lib/utils.js";
import { page } from "$app/state";

let {
	isAdmin,
}: {
	isAdmin?: boolean;
} = $props();

const baseMenuItems = [
	{
		label: "Knowledge Home",
		href: "/home",
		icon: Lightbulb,
		category: "main",
	},
	{
		label: "Your Trail",
		href: "/knowledge/trail",
		icon: Footprints,
		category: "main",
	},
	{
		label: "Search",
		href: "/search",
		icon: Search,
		category: "main",
	},
	{
		label: "Dashboard",
		href: "/dashboard",
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
				href: "/feeds",
				icon: Rss,
			},
			{
				label: "Visual Preview",
				href: "/feeds/visual-preview",
				icon: GalleryVerticalEnd,
			},
			{
				label: "Read History",
				href: "/feeds/viewed",
				icon: Eye,
			},
			{
				label: "Favorites",
				href: "/feeds/favorites",
				icon: Star,
			},
			{
				label: "Search",
				href: "/feeds/search",
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
				href: "/recap",
				icon: CalendarRange,
			},
			{
				label: "Morning Letter",
				href: "/recap/morning-letter",
				icon: Newspaper,
			},
			{
				label: "Evening Pulse",
				href: "/recap/evening-pulse",
				icon: Sparkles,
			},
			{
				label: "Job Status",
				href: "/recap/job-status",
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
				href: "/feeds/tag-trail",
				icon: Shuffle,
			},
			{
				label: "Tag Verse",
				href: "/feeds/tag-verse",
				icon: Orbit,
			},
			{
				label: "Tag Articles",
				href: "/articles/by-tag",
				icon: Tag,
			},
		],
	},
	{
		label: "Augur",
		category: "augur",
		icon: BirdIcon,
		children: [
			{
				label: "Ask Augur",
				href: "/augur",
				icon: BirdIcon,
			},
			{
				label: "History",
				href: "/augur/history",
				icon: MessagesSquare,
			},
		],
	},
	{
		label: "Acolyte Reports",
		href: "/acolyte",
		icon: ScrollText,
		category: "main",
	},
	{
		label: "Settings",
		category: "settings",
		icon: Settings,
		children: [
			{
				label: "Manage Feed Links",
				href: "/settings/feeds",
				icon: LinkIcon,
			},
			{
				label: "Notifications",
				href: "/settings/notifications",
				icon: Bell,
			},
		],
	},
	{
		label: "Admin",
		category: "admin",
		icon: Activity,
		children: [
			{
				label: "Knowledge Home Ops",
				href: "/admin/knowledge-home",
				icon: Activity,
			},
			{
				label: "System Monitor",
				href: "/admin/monitor",
				icon: Gauge,
			},
		],
	},
	{
		label: "Statistics",
		href: "/stats",
		icon: ChartBar,
		category: "main",
	},
];

const showAdmin = $derived(isAdmin ?? page.data?.userRole === "admin");

const menuItems = $derived(
	showAdmin
		? baseMenuItems
		: baseMenuItems.filter((item) => item.category !== "admin"),
);

let expandedSections = $state<string[]>([]);

$effect(() => {
	const path = page.url.pathname;
	for (const item of baseMenuItems) {
		if ("children" in item && item.children) {
			const matches = item.children.some((c) => path.startsWith(c.href));
			if (matches && !expandedSections.includes(item.category)) {
				expandedSections = [...expandedSections, item.category];
			}
		}
	}
});

function toggleSection(category: string) {
	if (expandedSections.includes(category)) {
		expandedSections = expandedSections.filter((c) => c !== category);
	} else {
		expandedSections = [...expandedSections, category];
	}
}

function isActive(href: string): boolean {
	return page.url.pathname === href || page.url.pathname.startsWith(`${href}?`);
}

function isParentActive(children?: { href: string }[]): boolean {
	if (!children) return false;
	return children.some((child) => page.url.pathname.startsWith(child.href));
}
</script>

<aside
	class="sticky top-0 self-start h-[100dvh] w-60 flex-shrink-0 border-r border-[var(--surface-border)] bg-[var(--surface-bg)] overflow-y-auto"
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
										<!-- The active link is distinguished only by colour and
										     weight, which a screen reader cannot perceive.
										     aria-current is what announces "you are here". -->
										<a
											href={child.href}
											aria-current={isActive(child.href) ? "page" : undefined}
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
							aria-current={isActive(item.href) ? "page" : undefined}
							class={cn(
								"flex items-center gap-2 px-3 py-2 text-sm font-medium transition-colors duration-200",
								isActive(item.href)
									? "bg-[var(--surface-hover)] text-[var(--accent-primary)]"
									: "text-[var(--text-primary)] hover:bg-[var(--surface-hover)]",
							)}
						>
							<item.icon class="h-4 w-4" />
							<span>{item.label}</span>
							{#if item.label === "Knowledge Home" && !isActive(item.href)}
								<span class="ml-auto text-xs text-[var(--accent-primary)]/60 font-normal">Today</span>
							{/if}
						</a>
					</li>
				{/if}
			{/each}
		</ul>
	</nav>
</aside>
