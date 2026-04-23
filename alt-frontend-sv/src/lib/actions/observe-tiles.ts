/**
 * observeTiles — Svelte 5 action that wraps a single IntersectionObserver plus
 * a MutationObserver around a container element. Every child that carries
 * `data-entry-key` is observed exactly once; `onObserve(entryKey)` fires when
 * the child becomes at least `threshold` visible.
 *
 * Used by /loop/+page.svelte to drive dwell-based OBSERVE transitions without
 * creating one observer per tile (the /loop foreground may rebalance at any
 * time per ADR-000831 §5.5).
 *
 * Throttling is intentionally left to the caller (see loop-observe-throttle)
 * so the observer stays pure transport for visibility events.
 */

export interface ObserveTilesOptions {
	onObserve: (entryKey: string) => void;
	rootMargin?: string;
	threshold?: number;
}

export function observeTiles(
	container: HTMLElement,
	options: ObserveTilesOptions,
): { update: (newOptions: ObserveTilesOptions) => void; destroy: () => void } {
	let current = options;
	const tracked = new WeakSet<Element>();

	const io = new IntersectionObserver(
		(entries) => {
			for (const e of entries) {
				if (!e.isIntersecting) continue;
				const key = (e.target as HTMLElement).dataset.entryKey;
				if (key) current.onObserve(key);
			}
		},
		{
			root: null,
			rootMargin: current.rootMargin ?? "0px",
			threshold: current.threshold ?? 0.5,
		},
	);

	function adopt(el: Element) {
		if (tracked.has(el)) return;
		tracked.add(el);
		io.observe(el);
	}

	function scan(root: ParentNode) {
		const tiles = root.querySelectorAll<HTMLElement>("[data-entry-key]");
		for (const t of tiles) adopt(t);
	}

	scan(container);

	const mo = new MutationObserver((records) => {
		for (const r of records) {
			r.addedNodes.forEach((n) => {
				if (!(n instanceof Element)) return;
				if (n.hasAttribute("data-entry-key")) adopt(n);
				scan(n);
			});
			r.removedNodes.forEach((n) => {
				if (!(n instanceof Element)) return;
				if (n.hasAttribute("data-entry-key")) {
					try {
						io.unobserve(n);
					} catch {
						// ignore: element may already be disconnected
					}
				}
			});
		}
	});
	mo.observe(container, { childList: true, subtree: true });

	return {
		update(newOptions) {
			current = newOptions;
		},
		destroy() {
			io.disconnect();
			mo.disconnect();
		},
	};
}
