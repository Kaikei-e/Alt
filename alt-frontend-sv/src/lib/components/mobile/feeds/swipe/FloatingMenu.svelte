<script lang="ts">
import {
	Activity,
	BirdIcon,
	CalendarRange,
	ChartBar,
	Eye,
	Globe,
	Home,
	Infinity as InfinityIcon,
	Link as LinkIcon,
	Menu,
	Newspaper,
	Plus,
	Rss,
	Search,
	Star,
	X,
} from "@lucide/svelte";
import { onMount } from "svelte";
import { browser } from "$app/environment";
import { page } from "$app/state";
import * as Accordion from "$lib/components/ui/accordion";
import { Button } from "$lib/components/ui/button";
import * as Sheet from "$lib/components/ui/sheet";
import { cn } from "$lib/utils";

let isOpen = $state(false);
let isPrefetched = $state(false);
let { class: className = "" } = $props();

// Prevent body scroll lock when dialog is closed (following React version pattern)
// This effect runs whenever isOpen changes and ensures body scroll is properly controlled
$effect(() => {
	if (!browser) return;

	// Use requestAnimationFrame to ensure this runs after bits-ui's internal scroll lock
	requestAnimationFrame(() => {
		if (isOpen) {
			// Prevent background scrolling when menu is open
			document.body.style.overflow = "hidden";
			document.body.style.position = "fixed";
			document.body.style.width = "100%";
		} else {
			// Ensure body scroll is enabled when dialog is closed
			// Override any scroll lock that bits-ui might have set
			document.body.style.overflow = "";
			document.body.style.position = "";
			document.body.style.width = "";
		}
	});

	// Cleanup function to ensure body scroll is restored
	return () => {
		requestAnimationFrame(() => {
			document.body.style.overflow = "";
			document.body.style.position = "";
			document.body.style.width = "";
		});
	};
});

const svBasePath = "/sv";

const menuItems = [
	{
		label: "View Feeds",
		href: `${svBasePath}/mobile/feeds`,
		category: "feeds",
		icon: Rss,
		description: "Browse all RSS feeds",
	},
	{
		label: "Swipe Mode",
		href: `${svBasePath}/mobile/feeds/swipe`,
		category: "feeds",
		icon: InfinityIcon,
		description: "Swipe through feeds",
	},
	{
		label: "Viewed Feeds",
		href: `${svBasePath}/mobile/feeds/viewed`,
		category: "feeds",
		icon: Eye,
		description: "Previously read feeds",
	},
	{
		label: "Favorite Feeds",
		href: `${svBasePath}/mobile/feeds/favorites`,
		category: "feeds",
		icon: Star,
		description: "Favorited articles",
	},
	{
		label: "Register Feed",
		href: `${svBasePath}/mobile/feeds/register`,
		category: "feeds",
		icon: Plus,
		description: "Add new RSS feed",
	},
	{
		label: "Manage Feeds Links",
		href: `${svBasePath}/mobile/feeds/manage`,
		category: "feeds",
		icon: LinkIcon,
		description: "Add or remove your registered RSS sources",
	},
	{
		label: "Search Feeds",
		href: `${svBasePath}/mobile/feeds/search`,
		category: "feeds",
		icon: Search,
		description: "Find specific feeds",
	},
	{
		label: "Ask Augur",
		href: `${svBasePath}/mobile/retrieve/ask-augur`,
		category: "augur",
		icon: BirdIcon,
		description: "Chat with your knowledge base",
	},
	{
		label: "7-Day Recap",
		href: `${svBasePath}/mobile/recap/7days`,
		category: "recap",
		icon: CalendarRange,
		description: "Review the weekly highlights",
	},
	{
		label: "Morning Letter",
		href: `${svBasePath}/mobile/recap/morning-letter`,
		category: "recap",
		icon: Newspaper,
		description: "Today's overnight updates",
	},
	{
		label: "Job Status",
		href: `${svBasePath}/mobile/recap/job-status`,
		category: "recap",
		icon: Activity,
		description: "Monitor recap job progress",
	},
	{
		label: "View Articles",
		href: `${svBasePath}/mobile/articles/view`,
		category: "articles",
		icon: Newspaper,
		description: "Browse all articles",
	},
	{
		label: "Search Articles",
		href: `${svBasePath}/mobile/articles/search`,
		category: "articles",
		icon: Search,
		description: "Search through articles",
	},
	{
		label: "View Stats",
		href: `${svBasePath}/mobile/feeds/stats`,
		category: "other",
		icon: ChartBar,
		description: "Analytics & insights",
	},
	{
		label: "Home",
		href: `${svBasePath}/`,
		category: "other",
		icon: Home,
		description: "Return to dashboard",
	},
	{
		label: "Manage Domains",
		href: `${svBasePath}/admin/scraping-domains`,
		category: "other",
		icon: Globe,
		description: "Manage scraping domains",
	},
];

const categories = [
	{
		title: "Feeds",
		items: menuItems.filter((i) => i.category === "feeds"),
		icon: Rss,
	},
	{
		title: "Recap",
		items: menuItems.filter((i) => i.category === "recap"),
		icon: CalendarRange,
	},
	{
		title: "Articles",
		items: menuItems.filter((i) => i.category === "articles"),
		icon: Newspaper,
	},
	{
		title: "Augur",
		items: menuItems.filter((i) => i.category === "augur"),
		icon: BirdIcon,
		description: "Chat with your knowledge base",
	},
	{
		title: "Other",
		items: menuItems.filter((i) => i.category === "other"),
		icon: Star,
	},
];

function handleNavigate() {
	isOpen = false;
}

function isActiveMenuItem(href: string): boolean {
	return page.url.pathname === href;
}

onMount(() => {
	if (isPrefetched) return;

	const prefetch = () => {
		isPrefetched = true;
	};

	if ("requestIdleCallback" in window) {
		window.requestIdleCallback(prefetch);
	} else {
		setTimeout(prefetch, 0);
	}
});
</script>

<Sheet.Root bind:open={isOpen}>
	{#if !isOpen}
		<Sheet.Trigger
			class={cn(
				"fixed bottom-6 right-6 z-[1000] h-12 w-12 rounded-full border-2 border-[var(--text-primary)] bg-[var(--bg-surface)] text-[var(--text-primary)] shadow-[var(--shadow-glass)] backdrop-blur-md transition-all duration-300 hover:scale-105 hover:rotate-90 hover:bg-[var(--bg-surface-hover)] hover:border-[var(--accent-primary)] active:scale-95 active:rotate-90 inline-flex shrink-0 items-center justify-center focus-visible:outline-none outline-none disabled:pointer-events-none disabled:opacity-60",
				className,
			)}
			aria-label="Open floating menu"
		>
			<Menu class="h-5 w-5 relative z-[1]" />
		</Sheet.Trigger>
	{/if}
	<Sheet.Content
		side="bottom"
		class="max-h-[90vh] min-h-[70vh] rounded-t-[32px] border-t border-[var(--border-glass)] text-[var(--text-primary)] shadow-[0_-10px_40px_rgba(0,0,0,0.2)] backdrop-blur-[20px] w-full max-w-full sm:max-w-full p-0 gap-0 flex flex-col overflow-hidden [&>button.ring-offset-background]:hidden"
		style="background: white !important; background-color: white !important;"
	>
		<Sheet.Header class="border-b border-[var(--border-glass)] px-6 pb-6 pt-6">
			<div class="flex items-center justify-between">
				<div class="flex gap-3">
					<div
						class="flex h-10 w-10 items-center justify-center rounded-xl border border-[var(--border-glass)] bg-[var(--bg-surface)]"
					>
						<Star class="h-5 w-5 text-[var(--accent-primary)]" />
					</div>
					<div class="text-left">
						<Sheet.Title class="text-xl font-bold text-[var(--text-primary)]">
							Navigation
						</Sheet.Title>
						<Sheet.Description class="text-sm text-[var(--text-secondary)]">
							Quick access to all features
						</Sheet.Description>
					</div>
				</div>
			</div>
		</Sheet.Header>
		<div
			class="overflow-y-auto px-6 py-6 pb-[calc(1.5rem+env(safe-area-inset-bottom,0px))]"
		>
			<Accordion.Root type="multiple" class="w-full" value={["item-0"]}>
				{#each categories as cat, idx}
					<Accordion.Item value={`item-${idx}`} class="mb-4 border-none">
						<Accordion.Trigger
							class="flex w-full items-center justify-between rounded-2xl px-4 py-4 hover:bg-[var(--bg-surface-hover)] hover:no-underline data-[state=open]:bg-[var(--bg-surface)] transition-all duration-200"
						>
							<div class="flex items-center gap-4">
								<div
									class="text-[var(--text-secondary)] group-data-[state=open]:text-[var(--accent-primary)] transition-colors duration-200"
								>
									<cat.icon class="h-4 w-4" />
								</div>
								<span class="text-lg font-semibold text-[var(--text-primary)]">
									{cat.title}
								</span>
							</div>
						</Accordion.Trigger>
						<Accordion.Content class="pt-2 px-2 pb-2">
							<div class="flex flex-col gap-1">
								{#each cat.items as item}
									{@const active = isActiveMenuItem(item.href)}
									<a
										href={item.href}
										onclick={handleNavigate}
										class="block rounded-xl p-3 transition-all duration-200 hover:bg-[var(--bg-surface-hover)] {active
											? 'bg-[var(--bg-surface-active)]'
											: 'bg-transparent'}"
									>
										<div class="flex items-center gap-3">
											<div
												class={active
													? "text-[var(--accent-primary)]"
													: "text-[var(--text-secondary)]"}
											>
												<item.icon class="h-4 w-4" />
											</div>
											<div class="flex-1">
												<div
													class="text-sm font-medium {active
														? 'text-[var(--text-primary)] font-semibold'
														: 'text-[var(--text-primary)]'}"
												>
													{item.label}
												</div>
												{#if item.description}
													<div
														class="mt-0.5 text-xs text-[var(--text-secondary)]"
													>
														{item.description}
													</div>
												{/if}
											</div>
										</div>
									</a>
								{/each}
							</div>
						</Accordion.Content>
					</Accordion.Item>
				{/each}
			</Accordion.Root>
		</div>
		<Sheet.Close
			class="absolute right-6 top-6 h-10 w-10 rounded-full border-2 border-transparent bg-transparent text-[var(--text-primary)] hover:bg-[var(--bg-surface-hover)] hover:border-[var(--surface-border)] hover:rotate-90 hover:border-[var(--accent-primary)] transition-all duration-200 inline-flex shrink-0 items-center justify-center focus-visible:outline-none outline-none disabled:pointer-events-none disabled:opacity-60 border border-[var(--border-glass)] bg-[var(--bg-glass)] backdrop-blur-md"
			aria-label="Close dialog"
		>
			<X class="h-4 w-4" />
		</Sheet.Close>
	</Sheet.Content>
</Sheet.Root>
