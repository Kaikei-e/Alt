/**
 * Text utility functions for consistent text formatting across components
 */

/**
 * Truncates text to a specified length and adds ellipsis if needed
 * @param text - The text to truncate
 * @param maxLength - Maximum length before truncation (default: 200)
 * @param suffix - Suffix to add when truncated (default: "...")
 * @returns Truncated text with suffix if needed
 */
export const truncateText = (
  text: string,
  maxLength: number = 100,
  suffix: string = "..."
): string => {
  if (text.length <= maxLength) {
    return text;
  }

  return text.substring(0, maxLength) + suffix;
};

/**
 * Truncates feed description specifically
 * Uses the standard 200 character limit for feed descriptions
 * @param description - Feed description to truncate
 * @returns Truncated description
 */
export const truncateFeedDescription = (description: string): string => {
  return truncateText(description, 100);
};
