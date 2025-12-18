<script lang="ts">
  import { ArrowLeft, Home, RefreshCw, Trash2, Plus } from "@lucide/svelte";
  import { goto } from "$app/navigation";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import FloatingMenu from "$lib/components/mobile/feeds/swipe/FloatingMenu.svelte";
  import { feedUrlSchema } from "$lib/schema/validation/feedUrlSchema";
  import type { FeedLink } from "$lib/schema/feedLink";
  import {
    deleteFeedLinkClient,
    listFeedLinksClient,
    registerRssFeedClient,
  } from "$lib/api/client";
  import * as v from "valibot";

  interface PageData {
    feedLinks: FeedLink[];
    error?: string;
  }

  const { data }: { data: PageData } = $props();

  type ActionMessage = {
    type: "success" | "error";
    text: string;
  };

  let feedLinks = $state<FeedLink[]>([]);
  let isLoadingLinks = $state(false);
  let loadingError = $state<string | null>(null);
  let feedUrl = $state("");
  let validationError = $state<string | null>(null);
  let isSubmitting = $state(false);
  let selectedLink = $state<FeedLink | null>(null);
  let isDeleting = $state(false);
  let actionMessage = $state<ActionMessage | null>(null);
  let showAddForm = $state(false);
  let isDeleteDialogOpen = $state(false);

  // SSRから渡された初期データを一度だけ state に反映
  $effect(() => {
    feedLinks = data.feedLinks ?? [];
    loadingError = data.error ?? null;
  });

  const sortedLinks = $derived(
    [...feedLinks].sort((a, b) => a.url.localeCompare(b.url)),
  );

  function validateUrl(url: string): string | null {
    const trimmed = url.trim();
    if (!trimmed) return "Please enter the RSS URL.";

    const result = v.safeParse(feedUrlSchema, { feed_url: trimmed });
    if (!result.success) {
      return result.issues[0]?.message ?? "Invalid RSS URL.";
    }

    return null;
  }

  function resetForm() {
    feedUrl = "";
    validationError = null;
    showAddForm = false;
  }

  function handleUrlChange(event: Event) {
    const target = event.target as HTMLInputElement;
    feedUrl = target.value;
    validationError = null;
    actionMessage = null;
  }

  async function loadFeedLinks() {
    isLoadingLinks = true;
    loadingError = null;
    try {
      const links = await listFeedLinksClient();
      feedLinks = links;
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to load feed links.";
      loadingError = message;
    } finally {
      isLoadingLinks = false;
    }
  }

  async function handleSubmit(event: Event) {
    event.preventDefault();
    const error = validateUrl(feedUrl);
    if (error) {
      validationError = error;
      return;
    }

    isSubmitting = true;
    actionMessage = null;

    try {
      await registerRssFeedClient(feedUrl.trim());
      actionMessage = {
        type: "success",
        text: "Feed registered successfully.",
      };
      resetForm();
      await loadFeedLinks();
    } catch (err) {
      let message = "Failed to register feed.";
      if (err instanceof Error) {
        message = err.message;
      }
      actionMessage = { type: "error", text: message };
    } finally {
      isSubmitting = false;
    }
  }

  function handleDeleteRequested(link: FeedLink) {
    selectedLink = link;
    isDeleteDialogOpen = true;
  }

  async function handleDeleteConfirmed() {
    if (!selectedLink) return;

    isDeleting = true;
    try {
      await deleteFeedLinkClient(selectedLink.id);
      actionMessage = { type: "success", text: "Feed link deleted." };
      await loadFeedLinks();
    } catch (err) {
      let message = "Failed to delete feed link.";
      if (err instanceof Error) {
        message = err.message;
      }
      actionMessage = { type: "error", text: message };
    } finally {
      isDeleting = false;
      isDeleteDialogOpen = false;
      selectedLink = null;
    }
  }

  function handleDeleteCancel() {
    isDeleteDialogOpen = false;
    selectedLink = null;
  }

  function handleBackToFeeds() {
    void goto("/sv/mobile/feeds");
  }

  function handleBackToHome() {
    void goto("/sv");
  }
</script>

<div class="min-h-[100dvh] flex flex-col" style="background: var(--app-bg);">
  <div class="w-full max-w-xl mx-auto px-4 py-6 flex-1">
    <div class="flex flex-col gap-6">
      <!-- Header -->
      <section>
        <div class="flex items-center justify-between mb-3">
          <div class="flex items-center gap-3">
            <Button
              size="icon"
              variant="ghost"
              class="h-10 w-10 rounded-full"
              aria-label="Back to feeds list"
              onclick={handleBackToFeeds}
            >
              <ArrowLeft class="h-4 w-4" />
            </Button>
            <div class="flex flex-col">
              <h1
                class="text-lg font-semibold"
                style="color: var(--text-primary);"
              >
                Feed Management
              </h1>
              <p class="text-xs mt-1" style="color: var(--alt-text-secondary);">
                Add or remove the RSS sources that Alt will scan for your
                tenant.
              </p>
            </div>
          </div>
          <Button
            size="icon"
            variant="ghost"
            class="h-10 w-10 rounded-full"
            aria-label="Back to home"
            onclick={handleBackToHome}
          >
            <Home class="h-4 w-4" />
          </Button>
        </div>
      </section>

      <!-- Action Message -->
      {#if actionMessage}
        <section>
          <div
            class="rounded-xl p-4 text-sm"
            style="
							background: {actionMessage.type === 'success'
              ? 'var(--alt-success)'
              : 'var(--alt-error)'};
							color: white;
						"
          >
            <div class="flex gap-3 items-start">
              <div class="mt-0.5 shrink-0">
                {actionMessage.type === "success" ? "✓" : "✕"}
              </div>
              <div class="flex-1">
                <p class="font-semibold text-sm mb-1">
                  {actionMessage.type === "success" ? "Success" : "Error"}
                </p>
                <p class="text-xs">
                  {actionMessage.text}
                </p>
              </div>
            </div>
          </div>
        </section>
      {/if}

      <!-- Add Feed Button / Form -->
      <section class="space-y-3">
        {#if !showAddForm}
          <Button
            class="w-full rounded-full min-h-[48px] font-semibold text-base transition-all duration-200 hover:scale-[1.02] active:scale-[0.98]"
            style="
							background: var(--alt-primary);
							color: black;
						"
            onclick={() => (showAddForm = true)}
          >
            <div class="flex items-center justify-center gap-2">
              <Plus class="h-4 w-4" />
              <span>Add a new feed</span>
            </div>
          </Button>
        {:else}
          <div
            class="rounded-2xl border p-5"
            style="
							background: var(--surface-bg);
							border-color: var(--surface-border);
						"
          >
            <div class="flex items-center justify-between mb-4">
              <h2
                class="text-sm font-semibold"
                style="color: var(--text-primary);"
              >
                Add a new feed
              </h2>
              <Button
                variant="ghost"
                size="sm"
                onclick={() => {
                  resetForm();
                  actionMessage = null;
                }}
              >
                Cancel
              </Button>
            </div>
            <p class="text-xs mb-4" style="color: var(--text-muted);">
              Please enter the RSS URL. Alt will validate the URL before
              scheduling the fetch.
            </p>
            <form onsubmit={handleSubmit}>
              <div class="flex flex-col gap-4">
                <Input
                  type="url"
                  placeholder="https://example.com/feed.xml"
                  value={feedUrl}
                  oninput={handleUrlChange}
                  class="text-sm"
                  style="
										background: white;
										border-color: {validationError ? 'var(--alt-error)' : 'var(--surface-border)'};
									"
                />
                {#if validationError}
                  <p class="text-xs" style="color: var(--alt-error);">
                    {validationError}
                  </p>
                {/if}
                <Button
                  type="submit"
                  class="w-full rounded-full min-h-[44px] font-semibold text-sm transition-all duration-200 hover:scale-[1.02] active:scale-[0.98]"
                  style="
										background: var(--alt-primary);
										color: black;
									"
                  disabled={isSubmitting}
                >
                  {#if isSubmitting}
                    <span>Registering...</span>
                  {:else}
                    <span>Add feed</span>
                  {/if}
                </Button>
              </div>
            </form>
          </div>
        {/if}
      </section>

      <!-- Feed Links List -->
      <section>
        <div
          class="rounded-2xl border p-5"
          style="
						background: var(--surface-bg);
						border-color: var(--surface-border);
					"
        >
          <div class="flex items-center justify-between mb-4">
            <h2
              class="text-sm font-semibold"
              style="color: var(--text-primary);"
            >
              Registered feeds
            </h2>
            <Button
              variant="ghost"
              size="icon"
              class="h-8 w-8 rounded-full"
              aria-label="Refresh"
              onclick={() => loadFeedLinks()}
              disabled={isLoadingLinks}
            >
              {#if isLoadingLinks}
                <span
                  class="animate-spin h-4 w-4 border-2 border-current border-t-transparent rounded-full"
                ></span>
              {:else}
                <RefreshCw class="h-4 w-4" />
              {/if}
            </Button>
          </div>

          {#if isLoadingLinks}
            <div class="flex items-center justify-center py-10">
              <span
                class="animate-spin h-6 w-6 border-2 border-current border-t-transparent rounded-full"
              ></span>
            </div>
          {:else if loadingError}
            <div
              class="rounded-md p-3 text-xs"
              style="
								background: var(--alt-error);
								color: white;
							"
            >
              {loadingError}
            </div>
          {:else if sortedLinks.length === 0}
            <p
              class="text-sm text-center py-6"
              style="color: var(--text-muted);"
            >
              No feeds registered yet.
            </p>
          {:else}
            <div class="flex flex-col gap-3">
              {#each sortedLinks as link (link.id)}
                <div
                  class="flex items-center justify-between px-4 py-3 rounded-xl border"
                  style="
										background: var(--app-bg);
										border-color: var(--surface-border);
									"
                >
                  <p
                    class="text-sm font-medium truncate mr-3 flex-1"
                    style="color: var(--text-primary);"
                  >
                    {link.url}
                  </p>
                  <Button
                    variant="ghost"
                    size="icon"
                    class="h-9 w-9 rounded-full text-[var(--alt-error)]"
                    aria-label="Delete feed link"
                    onclick={() => handleDeleteRequested(link)}
                  >
                    <Trash2 class="h-4 w-4" />
                  </Button>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      </section>
    </div>
  </div>

  <!-- Delete Confirmation Dialog (simple overlay) -->
  {#if isDeleteDialogOpen && selectedLink}
    <div
      class="fixed inset-0 z-[1000] flex items-center justify-center px-4"
      style="
				background: rgba(0, 0, 0, 0.6);
				backdrop-filter: blur(8px);
			"
    >
      <div
        class="w-full max-w-sm rounded-2xl border p-5 space-y-4"
        style="
					background: var(--surface-bg);
					border-color: var(--surface-border);
				"
      >
        <h2 class="text-base font-semibold" style="color: var(--text-primary);">
          Delete feed link?
        </h2>
        <p class="text-sm" style="color: var(--text-primary);">
          <strong>{selectedLink.url}</strong>
          <br />
          Deleting this feed link will remove it from the registry and stop Alt from
          checking it. This action cannot be undone.
        </p>
        <div class="flex justify-end gap-3 pt-2">
          <Button variant="outline" size="sm" onclick={handleDeleteCancel}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            size="sm"
            onclick={handleDeleteConfirmed}
            disabled={isDeleting}
          >
            {#if isDeleting}
              Deleting...
            {:else}
              Delete
            {/if}
          </Button>
        </div>
      </div>
    </div>
  {/if}

  <!-- Floating Menu -->
  <FloatingMenu />
</div>
