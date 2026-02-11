/**
 * Infinite scroll action for Svelte 5
 *
 * Uses IntersectionObserver to detect when an element enters the viewport
 * and triggers a callback. This is a simpler alternative to complex $effect-based
 * implementations.
 *
 * @example
 * ```svelte
 * <div use:infiniteScroll={{ callback: loadMore, root: scrollContainer, disabled: !hasMore }}>
 *   Loading...
 * </div>
 * ```
 */

interface InfiniteScrollOptions {
	/** Callback function to execute when element intersects */
	callback: () => void | Promise<void>;
	/** Root element for intersection (null = viewport) */
	root?: HTMLElement | null;
	/** Margin around root for intersection detection */
	rootMargin?: string;
	/** Threshold for intersection (0.0 to 1.0) */
	threshold?: number;
	/** Disable the observer when true */
	disabled?: boolean;
}

export function infiniteScroll(
	element: HTMLElement,
	options: InfiniteScrollOptions,
): {
	update: (newOptions: InfiniteScrollOptions) => void;
	destroy: () => void;
} {
	let observer: IntersectionObserver | null = null;
	let currentOptions = options;
	let isSettingUp = false;

	const setupObserver = () => {
		// 再帰呼び出しを防ぐ
		if (isSettingUp) {
			return;
		}

		isSettingUp = true;

		try {
			if (observer) {
				observer.disconnect();
				observer = null;
			}

			if (currentOptions.disabled) {
				return;
			}

			observer = new IntersectionObserver(
				async (entries) => {
					const [entry] = entries;
					if (!entry?.isIntersecting) return;
					if (currentOptions.disabled) return;

					// Temporarily unobserve to prevent duplicate triggers
					if (observer && element) {
						observer.unobserve(element);
					}

					try {
						await currentOptions.callback();
					} catch (error) {
						console.error("[infiniteScroll] callback error:", error);
					} finally {
						// Re-observe after a short delay to allow DOM updates
						await new Promise((resolve) => requestAnimationFrame(resolve));
						// Check if observer still exists and element is still connected
						if (
							observer &&
							element &&
							element.isConnected &&
							!currentOptions.disabled
						) {
							try {
								observer.observe(element);
							} catch (err) {
								// Element may have been removed, ignore error
							}
						}
						// 再帰呼び出しを防ぐ: finally ブロック内では setupObserver() を呼ばない
						// 代わりに、update() メソッドで適切に処理される
					}
				},
				{
					root: currentOptions.root ?? null,
					rootMargin: currentOptions.rootMargin ?? "0px 0px 200px 0px",
					threshold: currentOptions.threshold ?? 0.1,
				},
			);

			observer.observe(element);
		} finally {
			isSettingUp = false;
		}
	};

	setupObserver();

	return {
		update(newOptions: InfiniteScrollOptions) {
			const wasDisabled = currentOptions.disabled;
			// Normalize root: undefined and null both mean viewport
			const currentRoot = currentOptions.root ?? null;
			const newRoot = newOptions.root ?? null;

			// Improved root comparison: check if both are null or both are the same element
			const rootChanged =
				(currentRoot === null && newRoot !== null) ||
				(currentRoot !== null && newRoot === null) ||
				(currentRoot !== null && newRoot !== null && currentRoot !== newRoot);
			const rootMarginChanged =
				currentOptions.rootMargin !== newOptions.rootMargin;
			const thresholdChanged =
				currentOptions.threshold !== newOptions.threshold;

			currentOptions = newOptions;

			// If disabled state changed or observer options changed, recreate observer
			if (
				wasDisabled !== newOptions.disabled ||
				rootChanged ||
				rootMarginChanged ||
				thresholdChanged
			) {
				setupObserver();
			} else if (
				!observer &&
				!newOptions.disabled &&
				element &&
				element.isConnected &&
				!isSettingUp
			) {
				// Observer was disconnected but should be enabled, recreate it
				// ただし、setupObserver() が実行中でない場合のみ
				setupObserver();
			}
		},
		destroy() {
			if (observer) {
				observer.disconnect();
				observer = null;
			}
		},
	};
}
