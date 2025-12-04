<script lang="ts">
import { resolve } from "$app/paths";
import { Button } from "$lib/components/ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardFooter,
	CardHeader,
	CardTitle,
} from "$lib/components/ui/card";
import type { PageData } from "./$types";

const { data }: { data: PageData } = $props();
const errorId = $derived(data.errorId);
const loginPath = resolve("/login");
const homePath = resolve("/");
</script>

<div class="flex items-center justify-center min-h-screen" style="background: var(--app-bg);">
	<Card class="w-[350px]">
		<CardHeader>
			<CardTitle>Error Occurred</CardTitle>
			<CardDescription>
				An error occurred during authentication. Please try again.
			</CardDescription>
		</CardHeader>
		<CardContent>
			{#if errorId}
				<div class="p-3 text-sm font-medium text-center" style="color: var(--text-muted);">
					Error ID: {errorId}
				</div>
			{/if}
			<div class="p-3 text-sm font-medium text-center" style="color: #dc2626;">
				Sorry, an error occurred during authentication. Please try again.
				<br />
				Please try logging in again.
			</div>
		</CardContent>
		<CardFooter class="flex flex-col gap-2">
			<Button href={loginPath} class="w-full">
				Go to Login Page
			</Button>
			<a href={homePath} class="text-sm hover:underline text-center" style="color: var(--text-muted);">
				Go to Home
			</a>
		</CardFooter>
	</Card>
</div>
