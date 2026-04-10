<script lang="ts">
import {
	Activity,
	BirdIcon,
	CalendarRange,
	ChartBar,
	Compass,
	Eye,
	Globe,
	Home,
	Image as ImageIcon,
	Infinity as InfinityIcon,
	Link as LinkIcon,
	Menu,
	Moon,
	Newspaper,
	Rss,
	Search,
	Shuffle,
	Star,
	X,
} from "@lucide/svelte";
import { onMount } from "svelte";
import { browser } from "$app/environment";
import { page } from "$app/state";
import * as Accordion from "$lib/components/ui/accordion";
import * as Sheet from "$lib/components/ui/sheet";
import { cn } from "$lib/utils";

let isOpen = $state(false);
let isPrefetched = $state(false);
let { class: className = "" } = $props();

$effect(() => {
	if (!browser) return;

	requestAnimationFrame(() => {
		if (isOpen) {
			document.body.style.overflow = "hidden";
			document.body.style.position = "fixed";
			document.body.style.width = "100%";
		} else {
			document.body.style.overflow = "";
			document.body.style.position = "";
			document.body.style.width = "";
		}
	});

	return () => {
		requestAnimationFrame(() => {
			document.body.style.overflow = "";
			document.body.style.position = "";
			document.body.style.width = "";
		});
	};
});

const svBasePath = "";

const menuItems = [
	{
		label: "View Feeds",
		href: `${svBasePath}/feeds`,
		category: "feeds",
		icon: Rss,
		description: "Browse all RSS feeds",
	},
	{
		label: "Swipe Mode",
		href: `${svBasePath}/feeds/swipe`,
		category: "feeds",
		icon: InfinityIcon,
		description: "Swipe through feeds",
	},
	{
		label: "Visual Preview",
		href: `${svBasePath}/feeds/swipe/visual-preview`,
		category: "feeds",
		icon: ImageIcon,
		description: "Swipe with thumbnail images",
	},
	{
		label: "Viewed Feeds",
		href: `${svBasePath}/feeds/viewed`,
		category: "feeds",
		icon: Eye,
		description: "Previously read feeds",
	},
	{
		label: "Favorite Feeds",
		href: `${svBasePath}/feeds/favorites`,
		category: "feeds",
		icon: Star,
		description: "Favorited articles",
	},
	{
		label: "Manage Feeds Links",
		href: `${svBasePath}/settings/feeds`,
		category: "feeds",
		icon: LinkIcon,
		description: "Add or remove your registered RSS sources",
	},
	{
		label: "Search Feeds",
		href: `${svBasePath}/feeds/search`,
		category: "feeds",
		icon: Search,
		description: "Find specific feeds",
	},
	{
		label: "Tag Trail",
		href: `${svBasePath}/feeds/tag-trail`,
		category: "explore",
		icon: Shuffle,
		description: "Discover feeds by exploring tags",
	},
	{
		label: "Ask Augur",
		href: `${svBasePath}/augur`,
		category: "augur",
		icon: BirdIcon,
		description: "Chat with your knowledge base",
	},
	{
		label: "3-Day Recap",
		href: `${svBasePath}/recap`,
		category: "recap",
		icon: CalendarRange,
		description: "Review recent highlights",
	},
	{
		label: "Morning Letter",
		href: `${svBasePath}/recap/morning-letter`,
		category: "recap",
		icon: Newspaper,
		description: "Today's overnight updates",
	},
	{
		label: "Evening Pulse",
		href: `${svBasePath}/recap/evening-pulse`,
		category: "recap",
		icon: Moon,
		description: "Tonight's key highlights",
	},
	{
		label: "Job Status",
		href: `${svBasePath}/recap/job-status`,
		category: "recap",
		icon: Activity,
		description: "Monitor recap job progress",
	},
	{
		label: "View Stats",
		href: `${svBasePath}/stats`,
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
		title: "Explore",
		items: menuItems.filter((i) => i.category === "explore"),
		icon: Compass,
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
				"menu-trigger",
				className,
			)}
			aria-label="Open floating menu"
		>
			<Menu size={18} />
		</Sheet.Trigger>
	{/if}
	<Sheet.Content
		side="bottom"
		class="max-h-[90vh] min-h-[70vh] w-full max-w-full sm:max-w-full p-0 gap-0 flex flex-col overflow-hidden [&>button.ring-offset-background]:hidden"
		style="background: var(--surface-bg) !important; border-top: 1px solid var(--surface-border); border-radius: 0;"
	>
		<Sheet.Header class="sheet-header">
			<div class="flex items-center justify-between">
				<div class="flex gap-3 items-center">
					<div class="header-icon-box">
						<Star size={16} class="header-icon" />
					</div>
					<div class="text-left">
						<Sheet.Title class="sheet-title">
							Navigation
						</Sheet.Title>
						<Sheet.Description class="sheet-description">
							Quick access to all features
						</Sheet.Description>
					</div>
				</div>
			</div>
		</Sheet.Header>
		<div class="sheet-body">
			<Accordion.Root type="multiple" class="w-full" value={["item-0"]}>
				{#each categories as cat, idx}
					<Accordion.Item value={`item-${idx}`} class="mb-3 border-none">
						<Accordion.Trigger
							class="accordion-trigger"
						>
							<div class="flex items-center gap-3">
								<div class="cat-icon">
									<cat.icon size={14} />
								</div>
								<span class="cat-title">
									{cat.title}
								</span>
							</div>
						</Accordion.Trigger>
						<Accordion.Content class="pt-1 px-2 pb-2">
							<div class="flex flex-col">
								{#each cat.items as item}
									{@const active = isActiveMenuItem(item.href)}
									<a
										href={item.href}
										onclick={handleNavigate}
										class="nav-item {active ? 'nav-item--active' : ''}"
									>
										<div class="flex items-center gap-3">
											<div class="nav-item-icon {active ? 'nav-item-icon--active' : ''}">
												<item.icon size={14} />
											</div>
											<div class="flex-1">
												<div class="nav-item-label {active ? 'nav-item-label--active' : ''}">
													{item.label}
												</div>
												{#if item.description}
													<div class="nav-item-desc">
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
			class="sheet-close"
			aria-label="Close dialog"
		>
			<X size={14} />
		</Sheet.Close>
	</Sheet.Content>
</Sheet.Root>

<style>
	/* ── Trigger ── */
	:global(.menu-trigger) {
		position: fixed;
		bottom: 1.5rem;
		right: 1.5rem;
		z-index: 1000;
		width: 48px;
		height: 48px;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		background: var(--surface-bg);
		border: 1.5px solid var(--alt-charcoal);
		color: var(--alt-charcoal);
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
		border-radius: 0;
		outline: none;
	}

	:global(.menu-trigger:active) {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	/* ── Sheet ── */
	:global(.sheet-header) {
		border-bottom: 1px solid var(--surface-border);
		padding: 1.5rem;
	}

	.header-icon-box {
		display: flex;
		width: 36px;
		height: 36px;
		align-items: center;
		justify-content: center;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
	}

	.header-icon-box :global(.header-icon) {
		color: var(--alt-primary);
	}

	:global(.sheet-title) {
		font-family: var(--font-display);
		font-size: 1.1rem;
		font-weight: 700;
		color: var(--alt-charcoal);
	}

	:global(.sheet-description) {
		font-family: var(--font-body);
		font-size: 0.82rem;
		color: var(--alt-slate);
	}

	.sheet-body {
		overflow-y: auto;
		padding: 1.5rem;
		padding-bottom: calc(1.5rem + env(safe-area-inset-bottom, 0px));
	}

	:global(.sheet-close) {
		position: absolute;
		right: 1.5rem;
		top: 1.5rem;
		width: 36px;
		height: 36px;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		background: transparent;
		border: 1px solid var(--surface-border);
		color: var(--alt-charcoal);
		cursor: pointer;
		transition: background 0.15s, border-color 0.15s;
		border-radius: 0;
		outline: none;
	}

	:global(.sheet-close:hover) {
		background: var(--surface-hover);
		border-color: var(--alt-charcoal);
	}

	/* ── Accordion ── */
	:global(.accordion-trigger) {
		display: flex;
		width: 100%;
		align-items: center;
		justify-content: space-between;
		padding: 0.75rem;
		border-radius: 0 !important;
		transition: background 0.15s;
	}

	:global(.accordion-trigger:hover) {
		background: var(--surface-hover);
	}

	:global(.accordion-trigger[data-state="open"]) {
		background: var(--surface-bg);
	}

	.cat-icon {
		color: var(--alt-slate);
		transition: color 0.15s;
	}

	.cat-title {
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--alt-charcoal);
	}

	/* ── Nav items ── */
	.nav-item {
		display: block;
		padding: 0.6rem 0.75rem;
		text-decoration: none;
		transition: background 0.15s;
	}

	.nav-item:hover {
		background: var(--surface-hover);
	}

	.nav-item--active {
		background: var(--surface-hover);
	}

	.nav-item-icon {
		color: var(--alt-slate);
	}

	.nav-item-icon--active {
		color: var(--alt-primary);
	}

	.nav-item-label {
		font-family: var(--font-body);
		font-size: 0.82rem;
		color: var(--alt-charcoal);
	}

	.nav-item-label--active {
		font-weight: 600;
	}

	.nav-item-desc {
		font-family: var(--font-body);
		font-size: 0.7rem;
		color: var(--alt-ash);
		margin-top: 0.15rem;
	}
</style>
