// See https://svelte.dev/docs/kit/types#app.d.ts
// for information about these interfaces
declare global {
	namespace App {
		// interface Error {}
		interface Locals {
			user: import("@ory/client").Identity | null;
			session: import("@ory/client").Session | null;
		}
		interface PageData {
			user: import("@ory/client").Identity | null;
		}
		// interface PageState {}
		// interface Platform {}
	}
}

export {};
