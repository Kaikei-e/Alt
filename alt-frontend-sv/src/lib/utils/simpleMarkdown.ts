/**
 * Simple Markdown Parser
 * Converts basic markdown syntax to HTML without third-party libraries.
 * Supported:
 * - Headers (# h1, ## h2, ### h3)
 * - Bold (**text**)
 * - Italic (*text*)
 * - Unordered Lists (- item)
 * - Ordered Lists (1. item)
 * - Code Blocks (```)
 * - Inline Code (`)
 * - Paragraphs
 */
export function parseMarkdown(text: string): string {
  if (!text) return '';

  const lines = text.split('\n');
  let html = '';
  let inCodeBlock = false;
  let inList = false;
  let listType: 'ul' | 'ol' | null = null;
  let paragraphBuffer: string[] = [];

  // Helper to flush current paragraph buffer
  const flushParagraph = () => {
    if (paragraphBuffer.length > 0) {
      if (inList) {
        // If we were in a list and hit a paragraph, close the list first?
        // Standard MD: List items can have paragraphs.
        // For simple parser: If we see a non-list item while in list,
        // usually it ends the list or continues the item.
        // Here we assume it ends the list if it's a distinct paragraph block.
        // But let's stick to the previous simple logic:
        // If we are strictly in a list, "paragraph" text usually means continuation of list item or sub-paragraph.
        // To simplify: close list if we hit a proper paragraph block that isn't indented.
        html += `</${listType}>\n`;
        inList = false;
        listType = null;
      }

      const content = paragraphBuffer.join('\n'); // Join with newline, let HTML handle distinct lines as space
      html += `<p class="mb-2 leading-relaxed">${parseInline(content)}</p>\n`;
      paragraphBuffer = [];
    }
  };

  for (let i = 0; i < lines.length; i++) {
    let line = lines[i];

    // Code Blocks
    if (line.trim().startsWith('```')) {
      flushParagraph();
      if (inCodeBlock) {
        html += '</code></pre>\n';
        inCodeBlock = false;
      } else {
        html += '<pre><code class="bg-muted p-2 rounded block whitespace-pre-wrap">';
        inCodeBlock = true;
      }
      continue;
    }

    if (inCodeBlock) {
      // Escape HTML in code blocks
      html += escapeHtml(line) + '\n';
      continue;
    }

    // Headers
    if (line.startsWith('# ')) {
      flushParagraph();
      if (inList) { html += `</${listType}>\n`; inList = false; listType = null; }
      html += `<h1 class="text-xl font-bold mt-4 mb-2">${parseInline(line.slice(2))}</h1>`;
      continue;
    }
    if (line.startsWith('## ')) {
      flushParagraph();
      if (inList) { html += `</${listType}>\n`; inList = false; listType = null; }
      html += `<h2 class="text-lg font-semibold mt-3 mb-2">${parseInline(line.slice(3))}</h2>`;
      continue;
    }
    if (line.startsWith('### ')) {
      flushParagraph();
      if (inList) { html += `</${listType}>\n`; inList = false; listType = null; }
      html += `<h3 class="text-md font-semibold mt-2 mb-1">${parseInline(line.slice(4))}</h3>`;
      continue;
    }

    // Lists
    const trimmed = line.trim();
    const isUnordered = trimmed.startsWith('- ');
    const isOrdered = /^\d+\.\s/.test(trimmed);

    if (isUnordered || isOrdered) {
      flushParagraph(); // Flush any pending paragraph before list item
      const currentListType = isUnordered ? 'ul' : 'ol';

      if (!inList || listType !== currentListType) {
        if (inList) html += `</${listType}>\n`;
        html += `<${currentListType} class="pl-5 mb-2 ${isUnordered ? 'list-disc' : 'list-decimal'}">`;
        inList = true;
        listType = currentListType;
      }

      // Extract content. preserve inner spaces.
      const content = isUnordered ? trimmed.slice(2) : trimmed.replace(/^\d+\.\s/, '');
      html += `<li>${parseInline(content)}</li>`;
      continue;
    }

    // Empty lines (paragraph separators)
    if (trimmed === '') {
      flushParagraph();
      if (inList) {
        // Empty line usually ends a tight list in simplified markdown,
        // or just separates items. Let's close list to be safe or keep it open?
        // Standard: Empty line separates list from next paragraph.
        html += `</${listType}>\n`;
        inList = false;
        listType = null;
      }
      continue;
    }

    // If we are here, it's a text line.
    // If inside a list but not a list item, strictly it should probably close the list
    // unless indented. Simplified parser: text line closes list.
    if (inList) {
      html += `</${listType}>\n`;
      inList = false;
      listType = null;
    }

    // Accumulate paragraph text
    paragraphBuffer.push(line);
  }

  // Final flush
  flushParagraph();

  if (inList) {
    html += `</${listType}>\n`;
  }
  if (inCodeBlock) {
    html += '</code></pre>\n';
  }

  return html;
}

function parseInline(text: string): string {
  // Bold
  text = text.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>');
  // Italic
  text = text.replace(/\*(.*?)\*/g, '<em>$1</em>');
  // Inline Code
  text = text.replace(/`([^`]+)`/g, '<code class="bg-muted px-1 rounded font-mono text-sm">$1</code>');
  return text;
}

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}
