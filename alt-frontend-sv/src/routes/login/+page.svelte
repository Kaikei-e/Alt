<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import {
    Card,
    CardContent,
    CardDescription,
    CardFooter,
    CardHeader,
    CardTitle,
  } from "$lib/components/ui/card";
  import type { PageData } from "./$types";

  let { data }: { data: PageData } = $props();
  let flow = $derived(data.flow);

  // Helper to find node by name
  function getNode(name: string) {
    return flow.ui.nodes.find((n) => (n.attributes as { name?: string }).name === name);
  }

  // Helper to get value from node
  function getValue(node: any) {
    return node?.attributes?.value || "";
  }

  // Helper to get error message
  function getError(node: any) {
    return node?.messages?.map((m: any) => m.text).join(" ");
  }
</script>

<div class="flex items-center justify-center min-h-screen" style="background: var(--app-bg);">
  <Card class="w-[350px]">
    <CardHeader>
      <CardTitle>Login</CardTitle>
      <CardDescription
        >Enter your credentials to access your account.</CardDescription
      >
    </CardHeader>
    <CardContent>
      <form action={flow.ui.action} method={(flow.ui.method || "post").toLowerCase() as "get" | "post"} class="space-y-4">
        <!-- CSRF Token -->
        {#if getNode("csrf_token")}
          {@const csrfNode = getNode("csrf_token")}
          <input type="hidden" name="csrf_token" value={getValue(csrfNode)} />
        {/if}

        <!-- Identifier (Email) -->
        {#if getNode("identifier")}
          {@const identifierNode = getNode("identifier")}
          <div class="space-y-2">
            <Label for="identifier">Email</Label>
            <Input
              id="identifier"
              name="identifier"
              type="email"
              value={getValue(identifierNode)}
              placeholder="m@example.com"
              required
            />
            {#if getError(identifierNode)}
              <p class="text-sm font-medium text-center" style="color: #dc2626;">{getError(identifierNode)}</p>
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

        <Button type="submit" class="w-full">Login</Button>
      </form>
    </CardContent>
    <CardFooter class="flex justify-center">
      <a href="/register" class="text-sm hover:underline" style="color: var(--text-muted);">
        Don't have an account? Register
      </a>
    </CardFooter>
  </Card>
</div>
