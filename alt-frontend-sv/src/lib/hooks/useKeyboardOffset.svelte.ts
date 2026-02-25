/**
 * Tracks the iOS software keyboard height via the VisualViewport API.
 * Returns a reactive offset (px) to push fixed bottom-positioned elements above the keyboard.
 *
 * @param isActive - reactive getter; listeners are only attached when true
 */
export function useKeyboardOffset(isActive: () => boolean) {
	let offset = $state(0);

	$effect(() => {
		if (!isActive()) {
			offset = 0;
			return;
		}

		const vv = window.visualViewport;
		if (!vv) return;

		function update() {
			// innerHeight = layout viewport (stable on iOS)
			// vv.height   = visual viewport (shrinks when keyboard opens)
			// vv.offsetTop = scroll offset of visual viewport within layout viewport
			offset = Math.max(
				0,
				Math.round(window.innerHeight - vv!.height - vv!.offsetTop),
			);
		}

		vv.addEventListener("resize", update);
		vv.addEventListener("scroll", update);
		update();

		return () => {
			vv.removeEventListener("resize", update);
			vv.removeEventListener("scroll", update);
			offset = 0;
		};
	});

	return {
		get value() {
			return offset;
		},
		/** bottom + max-height override inline style string. Empty when offset=0. */
		get style() {
			if (offset <= 0) return "";
			return `bottom: ${offset}px; max-height: min(70vh, calc(100vh - ${offset}px));`;
		},
	};
}
