import type { Identity } from "@ory/client";
import { page } from "$app/state";

class AuthStore {
	user = $state<Identity | null>(null);
	isAuthenticated = $derived(!!this.user);

	constructor() {
		// Sync with page data
		$effect.root(() => {
			$effect(() => {
				const data = page.data;
				if (data && data.user !== undefined) {
					this.user = data.user;
				}
			});
		});
	}

	setUser(user: Identity | null) {
		this.user = user;
	}
}

export const auth = new AuthStore();
