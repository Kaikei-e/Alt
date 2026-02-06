<script lang="ts">
import logo from "$lib/assets/favicon.svg";
import { Button } from "$lib/components/ui/button";
import { getAuthStore } from "$lib/stores/auth.svelte";

const auth = getAuthStore();
</script>

<div class="p-8 max-w-2xl mx-auto">
  <div class="flex flex-col items-center mb-6">
    <img src={logo} alt="Alt Logo" class="w-24 h-24 mb-4" />
    <h1 class="text-3xl font-bold">Alt: The AI Powered RSS Reader</h1>
  </div>

  <div class="p-6 border rounded-lg shadow-sm bg-card text-card-foreground">
    <h2 class="text-xl font-semibold mb-4">Authentication Status</h2>

    {#if auth.isAuthenticated}
      <div class="space-y-6">
        <div
          class="p-6 bg-gradient-to-br from-green-50 to-emerald-50 dark:from-green-900/20 dark:to-emerald-900/20 rounded-lg border border-green-200 dark:border-green-800"
        >
          <h3 class="text-2xl font-bold mb-2 text-primary">Welcome Back</h3>
          <p class="text-lg text-primary mb-6">
            Embark on a Journey of Discovery
          </p>
          <Button
            href="/sv/home"
            size="lg"
            class="w-full sm:w-auto"
            variant="secondary"
          >
            Explore Now →
          </Button>
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
