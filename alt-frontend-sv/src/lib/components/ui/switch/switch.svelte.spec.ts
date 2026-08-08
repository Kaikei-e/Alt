import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-svelte";

import Switch from "./switch.svelte";

describe("Switch", () => {
	it("exposes its state to assistive technology rather than by colour alone", () => {
		const screen = render(Switch, {
			props: { checked: true, label: "Recap finished" },
		});

		const control = screen.container.querySelector('[role="switch"]');
		expect(control?.getAttribute("aria-checked")).toBe("true");
		expect(control?.getAttribute("aria-label")).toBe("Recap finished");
	});

	it("reports the requested state, not the current one, when clicked", () => {
		const onchange = vi.fn();
		const screen = render(Switch, {
			props: { checked: false, label: "Recap finished", onchange },
		});

		screen.container
			.querySelector<HTMLButtonElement>('[role="switch"]')
			?.click();

		expect(onchange).toHaveBeenCalledWith(true);
	});

	it("swallows clicks while a change is in flight", () => {
		// Without this a second tap starts a second subscribe round trip, and the
		// two responses race to decide what the toggle finally reads.
		const onchange = vi.fn();
		const screen = render(Switch, {
			props: { checked: false, label: "Recap finished", busy: true, onchange },
		});

		screen.container
			.querySelector<HTMLButtonElement>('[role="switch"]')
			?.click();

		expect(onchange).not.toHaveBeenCalled();
	});

	it("swallows clicks while disabled", () => {
		const onchange = vi.fn();
		const screen = render(Switch, {
			props: {
				checked: false,
				label: "Recap finished",
				disabled: true,
				onchange,
			},
		});

		screen.container
			.querySelector<HTMLButtonElement>('[role="switch"]')
			?.click();

		expect(onchange).not.toHaveBeenCalled();
	});

	it("meets the 44px touch-target floor", () => {
		const screen = render(Switch, {
			props: { checked: false, label: "Recap finished" },
		});

		const control = screen.container.querySelector('[role="switch"]');
		expect(control).not.toBeNull();
		const box = (control as HTMLElement).getBoundingClientRect();

		// DESIGN_LANGUAGE.md: "Alt's baseline is 44px; never go below." The
		// existing settings/feeds toggle is 24px, which is why this is asserted
		// rather than assumed.
		expect(box.height).toBeGreaterThanOrEqual(44);
		expect(box.width).toBeGreaterThanOrEqual(44);
	});
});
