import { page } from "$app/stores";
import { get } from "svelte/store";
import type { Identity } from "@ory/client";

class AuthStore {
  user = $state<Identity | null>(null);
  isAuthenticated = $derived(!!this.user);

  constructor() {
    // Sync with page data
    $effect.root(() => {
      $effect(() => {
        const data = get(page).data;
        if (data.user !== undefined) {
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
