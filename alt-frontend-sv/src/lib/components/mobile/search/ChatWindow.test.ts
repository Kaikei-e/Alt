// @vitest-environment jsdom
import { render, fireEvent, screen, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import ChatWindow from './ChatWindow.svelte';
import * as streamingRenderer from '$lib/utils/streamingRenderer';

// Mock scrollIntoView
window.HTMLElement.prototype.scrollIntoView = vi.fn();

// Mock fetch
global.fetch = vi.fn();

// Mock streamingRenderer
vi.mock('$lib/utils/streamingRenderer', () => ({
  processStreamingText: vi.fn(),
}));

describe('ChatWindow', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders correctly', () => {
    const { getByPlaceholderText } = render(ChatWindow);
    expect(getByPlaceholderText('Type your message...')).toBeTruthy();
  });

  it('sends a message and displays user message', async () => {
    const { getByPlaceholderText, getByRole, getByText } = render(ChatWindow);
    const input = getByPlaceholderText('Type your message...') as HTMLInputElement;
    const button = getByRole('button', { name: /send/i });

    await fireEvent.input(input, { target: { value: 'Hello Augur' } });
    await fireEvent.click(button);

    expect(input.value).toBe(''); // Input cleared
    expect(getByText('Hello Augur')).toBeTruthy();
  });

  // More advanced tests for streaming would require mocking the stream reader
  // which is complex. For now, we verify basic interaction.
});
