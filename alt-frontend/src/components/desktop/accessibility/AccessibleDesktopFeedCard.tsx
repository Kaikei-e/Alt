"use client";

import { Box } from "@chakra-ui/react";
import type React from "react";
import { useRef } from "react";
import { DesktopFeedCard } from "@/components/desktop/timeline/DesktopFeedCard";
import { useFocusManagement } from "@/hooks/useFocusManagement";
import { useKeyboardNavigation } from "@/hooks/useKeyboardNavigation";
import type { DesktopFeed } from "@/types/desktop-feed";

interface AccessibleDesktopFeedCardProps {
  feed: DesktopFeed;
  index: number;
  total: number;
  onNavigate: (direction: "up" | "down") => void;
  onMarkAsRead: (feedId: string) => void;
  onToggleFavorite: (feedId: string) => void;
  onToggleBookmark: (feedId: string) => void;
  onReadLater: (feedId: string) => void;
  onViewArticle: (feedId: string) => void;
}

export const AccessibleDesktopFeedCard: React.FC<AccessibleDesktopFeedCardProps> = ({
  feed,
  index,
  total,
  onNavigate,
  ...props
}) => {
  const cardRef = useRef<HTMLDivElement>(null);
  const { focused, handleFocus, handleBlur } = useFocusManagement();

  useKeyboardNavigation({
    onArrowUp: () => index > 0 && onNavigate("up"),
    onArrowDown: () => index < total - 1 && onNavigate("down"),
    onEnter: () => props.onViewArticle(feed.id),
    onSpace: () => props.onMarkAsRead(feed.id),
    element: cardRef.current,
  });

  return (
    <Box
      ref={cardRef}
      role="article"
      aria-label={`Article ${index + 1} of ${total}: ${feed.title} by ${feed.metadata.source.name}`}
      aria-describedby={`feed-${feed.id}-description`}
      aria-posinset={index + 1}
      aria-setsize={total}
      tabIndex={0}
      onFocus={handleFocus}
      onBlur={handleBlur}
      data-testid={`accessible-feed-card-${feed.id}`}
      outline={focused ? "2px solid var(--accent-primary)" : "none"}
      outlineOffset="2px"
    >
      <div id={`feed-${feed.id}-description`} style={{ position: "absolute", left: "-9999px" }}>
        {feed.metadata.summary || feed.description}
        Published {feed.metadata.publishedAt} by {feed.metadata.source.name}. Reading time:{" "}
        {feed.metadata.readingTime} minutes.
        {feed.isRead ? "Already read." : "Unread."}
        Priority: {feed.metadata.priority}. Difficulty: {feed.metadata.difficulty}.
      </div>

      <DesktopFeedCard feed={feed} {...props} />

      {/* ライブリージョン for status updates */}
      <div
        style={{ position: "absolute", left: "-9999px" }}
        role="status"
        aria-live="polite"
        aria-atomic="true"
        id={`feed-${feed.id}-status`}
      />
    </Box>
  );
};
