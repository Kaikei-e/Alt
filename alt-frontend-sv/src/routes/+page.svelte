<script lang="ts">
  import { auth } from "$lib/stores/auth.svelte";
  import { Button } from "$lib/components/ui/button";
</script>

<div class="p-8 max-w-2xl mx-auto">
  <h1 class="text-3xl font-bold mb-6">Alt: The AI Powered RSS Reader</h1>

  <div class="p-6 border rounded-lg shadow-sm bg-card text-card-foreground">
    <h2 class="text-xl font-semibold mb-4">Authentication Status</h2>

    {#if auth.isAuthenticated}
      <div class="space-y-4">
        <div
          class="p-4 bg-green-50 dark:bg-green-900/20 text-green-700 dark:text-green-300 rounded-md"
        >
          <p class="font-medium">✅ Logged In</p>
        </div>

        <div class="grid grid-cols-[100px_1fr] gap-2 text-sm">
          <span class="font-medium text-muted-foreground">Email:</span>
          <span>{auth.user?.traits?.email || "Unknown"}</span>

          <span class="font-medium text-muted-foreground">ID:</span>
          <span class="font-mono text-xs">{auth.user?.id}</span>
        </div>

        <div class="pt-4 border-t">
          <form action="/logout" method="POST">
            <Button type="submit" variant="destructive">Logout</Button>
          </form>
        </div>
      </div>
    {:else}
      <div class="space-y-4">
        <div
          class="p-4 bg-yellow-50 dark:bg-yellow-900/20 text-yellow-700 dark:text-yellow-300 rounded-md"
        >
          <p class="font-medium">❌ Not Logged In</p>
        </div>

        <p class="text-muted-foreground">
          Please login or register to access your account.
        </p>

        <div class="flex gap-4 pt-2">
          <Button href="/login">Login</Button>
          <Button href="/register" variant="outline">Register</Button>
        </div>
      </div>
    {/if}
  </div>
</div>
