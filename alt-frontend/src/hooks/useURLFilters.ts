import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect } from "react";
import type { FilterState } from "@/types/desktop-feed";

/**
 * Custom hook for managing filter state in URL parameters
 * Allows for bookmarkable and shareable filter states
 */
export const useURLFilters = (
  filters: FilterState,
  onFilterChange: (filters: FilterState) => void,
  searchQuery: string,
  onSearchChange: (query: string) => void
) => {
  const router = useRouter();
  const searchParams = useSearchParams();

  // Load filters from URL on mount
  useEffect(() => {
    if (!searchParams) return;

    // Helper function to validate enum values
    const validateReadStatus = (value: string | null): FilterState["readStatus"] => {
      const validValues: FilterState["readStatus"][] = ["all", "read", "unread"];
      return validValues.includes(value as FilterState["readStatus"])
        ? (value as FilterState["readStatus"])
        : "all";
    };

    const validatePriority = (value: string | null): FilterState["priority"] => {
      const validValues: FilterState["priority"][] = ["all", "high", "medium", "low"];
      return validValues.includes(value as FilterState["priority"])
        ? (value as FilterState["priority"])
        : "all";
    };

    const validateTimeRange = (value: string | null): FilterState["timeRange"] => {
      const validValues: FilterState["timeRange"][] = ["all", "today", "week", "month"];
      return validValues.includes(value as FilterState["timeRange"])
        ? (value as FilterState["timeRange"])
        : "all";
    };

    const urlFilters: FilterState = {
      readStatus: validateReadStatus(searchParams.get("readStatus")),
      sources: searchParams.get("sources")?.split(",").filter(Boolean) || [],
      priority: validatePriority(searchParams.get("priority")),
      tags: searchParams.get("tags")?.split(",").filter(Boolean) || [],
      timeRange: validateTimeRange(searchParams.get("timeRange")),
    };

    const urlSearch = searchParams.get("search") || "";

    // Only update if URL has different values
    const filtersChanged =
      urlFilters.readStatus !== filters.readStatus ||
      JSON.stringify(urlFilters.sources) !== JSON.stringify(filters.sources) ||
      urlFilters.priority !== filters.priority ||
      JSON.stringify(urlFilters.tags) !== JSON.stringify(filters.tags) ||
      urlFilters.timeRange !== filters.timeRange;

    const searchChanged = urlSearch !== searchQuery;

    if (filtersChanged) {
      onFilterChange(urlFilters);
    }

    if (searchChanged) {
      onSearchChange(urlSearch);
    }
  }, []); // eslint-disable-line react-hooks/exhaustive-deps -- Only run on mount

  // Update URL when filters change
  const updateURL = useCallback(
    (newFilters: FilterState, newSearch: string) => {
      const params = new URLSearchParams();

      // Add non-default filter values to URL
      if (newFilters.readStatus !== "all") {
        params.set("readStatus", newFilters.readStatus);
      }

      if (newFilters.sources.length > 0) {
        params.set("sources", newFilters.sources.join(","));
      }

      if (newFilters.priority !== "all") {
        params.set("priority", newFilters.priority);
      }

      if (newFilters.tags.length > 0) {
        params.set("tags", newFilters.tags.join(","));
      }

      if (newFilters.timeRange !== "all") {
        params.set("timeRange", newFilters.timeRange);
      }

      if (newSearch.trim()) {
        params.set("search", newSearch);
      }

      // Update URL without triggering a navigation
      const newURL = params.toString() ? `?${params.toString()}` : "/desktop/feeds";
      router.replace(newURL, { scroll: false });
    },
    [router]
  );

  // Debounced URL update to avoid too many history entries
  useEffect(() => {
    const timeoutId = setTimeout(() => {
      updateURL(filters, searchQuery);
    }, 500); // 500ms delay

    return () => clearTimeout(timeoutId);
  }, [filters, searchQuery, updateURL]);

  // Function to share current filter state
  const shareFilters = useCallback(() => {
    const currentURL = window.location.href;

    if (navigator.share) {
      navigator.share({
        title: "Filtered Feeds",
        text: "Check out these filtered feeds",
        url: currentURL,
      });
    } else {
      // Fallback: copy to clipboard
      navigator.clipboard?.writeText(currentURL).then(() => {});
    }
  }, []);

  // Function to clear all filters (including URL)
  const clearAllFilters = useCallback(() => {
    const defaultFilters: FilterState = {
      readStatus: "all",
      sources: [],
      priority: "all",
      tags: [],
      timeRange: "all",
    };

    onFilterChange(defaultFilters);
    onSearchChange("");
    router.replace("/desktop/feeds", { scroll: false });
  }, [onFilterChange, onSearchChange, router]);

  return {
    shareFilters,
    clearAllFilters,
  };
};
