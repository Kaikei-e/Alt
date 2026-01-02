<script lang="ts">
import type { UiNode } from "@ory/client";
import { Button } from "$lib/components/ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardFooter,
	CardHeader,
	CardTitle,
} from "$lib/components/ui/card";
import { Input } from "$lib/components/ui/input";
import { Label } from "$lib/components/ui/label";
import type { PageData } from "./$types";

const { data }: { data: PageData } = $props();
const flow = $derived(data.flow);

// Helper to find node by name
function getNode(name: string): UiNode | undefined {
	return flow.ui.nodes.find(
		(n) => (n.attributes as { name?: string }).name === name,
	);
}

// Helper to get value from node
function getValue(node: UiNode | undefined): string {
	return (node?.attributes as { value?: string })?.value || "";
}

// Helper to get error message
function getError(node: UiNode | undefined): string {
	return node?.messages?.map((m) => m.text).join(" ") || "";
}
</script>

<div class="flex items-center justify-center min-h-screen" style="background: var(--app-bg);">
  <Card class="w-[350px]">
    <CardHeader>
      <CardTitle>Register</CardTitle>
      <CardDescription>Create a new account.</CardDescription>
    </CardHeader>
    <CardContent>
      <form action={flow.ui.action} method={(flow.ui.method || "post").toLowerCase() as "get" | "post"} class="space-y-4">
        <!-- CSRF Token -->
        {#if getNode("csrf_token")}
          {@const csrfNode = getNode("csrf_token")}
          <input type="hidden" name="csrf_token" value={getValue(csrfNode)} />
        {/if}

        <!-- Method: password strategy -->
        <input type="hidden" name="method" value="password" />

        <!-- Email -->
        {#if getNode("traits.email")}
          {@const emailNode = getNode("traits.email")}
          <div class="space-y-2">
            <Label for="traits.email">Email</Label>
            <Input
              id="traits.email"
              name="traits.email"
              type="email"
              value={getValue(emailNode)}
              placeholder="m@example.com"
              required
            />
            {#if getError(emailNode)}
              <p class="text-sm font-medium text-center" style="color: #dc2626;">{getError(emailNode)}</p>
            {/if}
          </div>
        {/if}

        <!-- Password -->
        {#if getNode("password")}
          {@const passwordNode = getNode("password")}
          <div class="space-y-2">
            <Label for="password">Password</Label>
            <Input id="password" name="password" type="password" required />
            {#if getError(passwordNode)}
              <p class="text-sm font-medium text-center" style="color: #dc2626;">{getError(passwordNode)}</p>
            {/if}
          </div>
        {/if}

        <!-- General Errors -->
        {#if flow.ui.messages}
          {#each flow.ui.messages as message}
            <div
              class="p-3 text-sm font-medium text-center"
              style="color: #dc2626;"
            >
              {message.text}
            </div>
          {/each}
        {/if}

        <Button type="submit" class="w-full">Register</Button>
      </form>
    </CardContent>
    <CardFooter class="flex justify-center">
      <a href="/login" class="text-sm hover:underline" style="color: var(--text-muted);">
        Already have an account? Login
      </a>
    </CardFooter>
  </Card>
</div>
