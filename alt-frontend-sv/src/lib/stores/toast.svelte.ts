export interface ToastItem {
	id: string;
	kind: "success" | "error" | "info";
	message: string;
}

export function useToastStore() {
	let items = $state<ToastItem[]>([]);

	function push(
		message: string,
		kind: ToastItem["kind"] = "info",
		timeoutMs = 2000,
	) {
		const id = crypto.randomUUID();
		items = [...items, { id, kind, message }];
		if (timeoutMs > 0) {
			setTimeout(() => {
				remove(id);
			}, timeoutMs);
		}
		return id;
	}

	function remove(id: string) {
		items = items.filter((item) => item.id !== id);
	}

	return {
		get items() {
			return items;
		},
		push,
		remove,
	};
}
