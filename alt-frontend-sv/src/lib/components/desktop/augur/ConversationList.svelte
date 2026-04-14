<script lang="ts">
import type { AugurConversationSummary } from "$lib/connect";
import { MessagesSquare, Trash2 } from "@lucide/svelte";

type Props = {
	conversations: AugurConversationSummary[];
	isLoading: boolean;
	errorMessage: string;
	hasMore: boolean;
	onOpen: (id: string) => void;
	onDelete: (id: string) => Promise<void> | void;
	onLoadMore: () => Promise<void> | void;
	onStartNew: () => void;
};

let {
	conversations,
	isLoading,
	errorMessage,
	hasMore,
	onOpen,
	onDelete,
	onLoadMore,
	onStartNew,
}: Props = $props();

// Alt-Paper dateline: small-caps, abbreviated month + day, monospace time.
function formatDateline(date: Date | null): string {
	if (!date) return "";
	const months = [
		"JAN",
		"FEB",
		"MAR",
		"APR",
		"MAY",
		"JUN",
		"JUL",
		"AUG",
		"SEP",
		"OCT",
		"NOV",
		"DEC",
	];
	const m = months[date.getMonth()];
	const d = String(date.getDate()).padStart(2, "0");
	const hh = String(date.getHours()).padStart(2, "0");
	const mm = String(date.getMinutes()).padStart(2, "0");
	return `${m} ${d} · ${hh}:${mm}`;
}

async function handleDelete(event: MouseEvent, id: string) {
	event.stopPropagation();
	try {
		await onDelete(id);
	} catch {
		/* surfaced via errorMessage */
	}
}

function handleKeydown(event: KeyboardEvent, id: string) {
	if (event.key === "Enter" || event.key === " ") {
		event.preventDefault();
		onOpen(id);
	}
}
</script>

<section class="conversation-list">
  <header class="list-header">
    <div class="header-stack">
      <span class="kicker">Ask Augur · History</span>
      <h1 class="title">Conversations</h1>
      <p class="subtitle">
        Every chat you&rsquo;ve had with Augur. Select one to continue where you
        left off, or start a new line of inquiry.
      </p>
    </div>
    <button type="button" class="new-button" onclick={onStartNew}>
      New Chat
    </button>
  </header>

  <div class="masthead-rule" aria-hidden="true"></div>

  {#if errorMessage}
    <p class="error-message" role="alert">{errorMessage}</p>
  {/if}

  {#if conversations.length === 0 && !isLoading}
    <div class="empty-state">
      <span class="empty-ornament" aria-hidden="true">◆</span>
      <p class="empty-heading">No conversations yet</p>
      <p class="empty-body">
        Questions you ask Augur will be stored here so you can retrace threads
        later.
      </p>
      <button type="button" class="new-button" onclick={onStartNew}>
        Ask Augur
      </button>
    </div>
  {:else}
    <ul class="items">
      {#each conversations as conv (conv.id)}
        <li class="item-wrap">
          <div
            class="item"
            role="button"
            tabindex="0"
            onclick={() => onOpen(conv.id)}
            onkeydown={(e) => handleKeydown(e, conv.id)}
          >
            <div class="item-main">
              <p class="item-title">{conv.title || "Untitled chat"}</p>
              {#if conv.lastMessagePreview}
                <p class="item-preview">{conv.lastMessagePreview}</p>
              {/if}
              <p class="item-meta">
                <span class="meta-date">
                  {formatDateline(conv.lastActivityAt ?? conv.createdAt)}
                </span>
                <span class="meta-sep" aria-hidden="true">·</span>
                <span class="meta-count">
                  <MessagesSquare size={12} strokeWidth={1.8} />
                  {conv.messageCount}
                  {conv.messageCount === 1 ? "turn" : "turns"}
                </span>
              </p>
            </div>
            <button
              type="button"
              class="delete-button"
              aria-label="Delete conversation"
              onclick={(e) => handleDelete(e, conv.id)}
            >
              <Trash2 size={16} strokeWidth={1.8} />
            </button>
          </div>
        </li>
      {/each}
    </ul>

    {#if hasMore}
      <div class="load-more-wrap">
        <button
          type="button"
          class="load-more"
          disabled={isLoading}
          onclick={() => onLoadMore()}
        >
          {isLoading ? "Loading…" : "Load more"}
        </button>
      </div>
    {/if}
  {/if}
</section>

<style>
  .conversation-list {
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
    max-width: 64rem;
    margin: 0 auto;
    padding: 1.5rem 1.25rem 3rem;
  }

  .list-header {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 1.5rem;
  }

  .header-stack {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    max-width: 42rem;
  }

  .kicker {
    font-family: var(--font-mono);
    font-size: 0.7rem;
    letter-spacing: 0.24em;
    text-transform: uppercase;
    color: var(--text-muted);
  }

  .title {
    font-family: var(--font-display, "Playfair Display", serif);
    font-size: clamp(1.75rem, 2.6vw, 2.4rem);
    line-height: 1.15;
    font-weight: 900;
    letter-spacing: -0.02em;
    color: var(--alt-charcoal, var(--text-primary));
    margin: 0;
  }

  .subtitle {
    font-family: var(--font-body);
    font-size: 0.9rem;
    line-height: 1.55;
    color: var(--text-secondary);
    margin: 0;
  }

  .masthead-rule {
    height: 1px;
    background: var(--surface-border);
    margin-top: 0.25rem;
  }

  .new-button {
    font-family: var(--font-mono);
    font-size: 0.72rem;
    letter-spacing: 0.18em;
    text-transform: uppercase;
    padding: 0.55rem 1rem;
    background: transparent;
    color: var(--text-primary);
    border: 1px solid var(--surface-border);
    cursor: pointer;
    transition: background 150ms ease;
    white-space: nowrap;
  }

  .new-button:hover {
    background: var(--surface-hover);
  }

  .error-message {
    font-family: var(--font-body);
    font-size: 0.85rem;
    color: #b91c1c;
    margin: 0;
  }

  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.6rem;
    padding: 3rem 1rem;
    text-align: center;
  }

  .empty-ornament {
    font-size: 1.1rem;
    color: var(--accent-primary, var(--text-muted));
    margin-bottom: 0.25rem;
  }

  .empty-heading {
    font-family: var(--font-display, "Playfair Display", serif);
    font-weight: 700;
    font-size: 1.25rem;
    letter-spacing: -0.01em;
    margin: 0;
    color: var(--alt-charcoal, var(--text-primary));
  }

  .empty-body {
    font-family: var(--font-body);
    font-size: 0.88rem;
    line-height: 1.6;
    color: var(--text-secondary);
    max-width: 28rem;
    margin: 0;
  }

  .items {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
  }

  .item-wrap + .item-wrap {
    border-top: 1px solid var(--surface-border);
  }

  .item {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
    padding: 1rem 0.6rem 1rem 1rem;
    cursor: pointer;
    border-left: 2px solid transparent;
    transition: background 120ms ease, border-color 120ms ease;
  }

  .item:hover,
  .item:focus-visible {
    background: var(--surface-hover);
    border-left-color: var(--accent-primary);
    outline: none;
  }

  .item-main {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    flex: 1;
    min-width: 0;
  }

  .item-title {
    font-family: var(--font-display, "Playfair Display", serif);
    font-weight: 700;
    font-size: 1.05rem;
    line-height: 1.3;
    letter-spacing: -0.01em;
    color: var(--alt-charcoal, var(--text-primary));
    margin: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .item-preview {
    font-family: var(--font-body);
    font-size: 0.87rem;
    line-height: 1.45;
    color: var(--text-secondary);
    margin: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .item-meta {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-family: var(--font-mono);
    font-size: 0.7rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--text-muted);
    margin: 0.15rem 0 0;
  }

  .meta-count {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
  }

  .meta-sep {
    opacity: 0.5;
  }

  .delete-button {
    background: transparent;
    border: 1px solid transparent;
    color: var(--text-muted);
    padding: 0.35rem;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border-radius: 0;
    transition: color 120ms ease, border-color 120ms ease;
  }

  .delete-button:hover {
    color: var(--text-primary);
    border-color: var(--surface-border);
  }

  .load-more-wrap {
    display: flex;
    justify-content: center;
    padding: 1.25rem 0;
  }

  .load-more {
    font-family: var(--font-mono);
    font-size: 0.7rem;
    letter-spacing: 0.18em;
    text-transform: uppercase;
    padding: 0.5rem 1.25rem;
    background: transparent;
    color: var(--text-primary);
    border: 1px solid var(--surface-border);
    cursor: pointer;
  }

  .load-more:disabled {
    opacity: 0.6;
    cursor: default;
  }

  @media (max-width: 48rem) {
    .list-header {
      flex-direction: column;
      align-items: flex-start;
    }

    .item-title {
      white-space: normal;
    }
  }
</style>
