import { render as rtlRender } from "@testing-library/svelte";
import type { ComponentType } from "svelte";

type RenderOptions<ComponentProps extends Record<string, any> = Record<string, any>> =
	Parameters<typeof rtlRender>[1] & {
		props?: ComponentProps;
	};

export function renderWithProps<ComponentProps extends Record<string, any>>(
	component: ComponentType,
	options?: RenderOptions<ComponentProps>,
) {
	return rtlRender(component, options);
}
