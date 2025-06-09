import { useState } from "react";
import {
  SearchQuery,
  searchQuerySchema,
} from "@/schema/validation/searchQuery";
import { feedsApi } from "@/lib/api";
import { BackendFeedItem } from "@/schema/feed";
import { transformFeedSearchResult } from "@/lib/utils/transformFeedSearchResult";
import * as v from "valibot";

interface SearchWindowProps {
  searchQuery: SearchQuery;
  setSearchQuery: (query: SearchQuery) => void;
  feedResults: BackendFeedItem[];
  setFeedResults: (results: BackendFeedItem[]) => void;
}

const SearchWindow = ({
  searchQuery,
  setSearchQuery,
  feedResults,
  setFeedResults,
}: SearchWindowProps) => {
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSearch = async () => {
    setIsLoading(true);
    setError(null);

    try {
      // Validate search query on client side
      const validationResult = v.safeParse(searchQuerySchema, {
        query: searchQuery.query,
      });

      if (!validationResult.success) {
        setError("Enter a valid search query");
        return;
      }

      const results = await feedsApi.searchFeeds(searchQuery.query);

      if (results.error) {
        setError(results.error);
        return;
      }

      const transformedResults = transformFeedSearchResult(results);

      // Filter results based on search query (case-insensitive)
      const filteredResults = transformedResults.filter(
        (feed) =>
          feed.title.toLowerCase().includes(searchQuery.query.toLowerCase()) ||
          feed.description
            .toLowerCase()
            .includes(searchQuery.query.toLowerCase()),
      );

      setFeedResults(filteredResults);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Search failed");
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div data-testid="search-window">
      <input
        data-testid="search-input"
        type="text"
        placeholder="Search for feeds"
        value={searchQuery.query}
        onChange={(e) => setSearchQuery({ query: e.target.value })}
      />
      <button onClick={handleSearch} disabled={isLoading}>
        {isLoading ? "Searching..." : "Search"}
      </button>
      {error && <div style={{ color: "red" }}>{error}</div>}
      <ul>
        {feedResults.map((feedResult, index) => {
          return (
            <li key={`${feedResult.title}-${feedResult.title}-${index}`}>
              <h2>{feedResult.title}</h2>
              <p>{feedResult.description}</p>
              <p>{feedResult.published}</p>
              <p>
                {feedResult.authors && feedResult.authors.length > 0
                  ? String(
                      feedResult.authors
                        .map((author) => author.name)
                        .join(", "),
                    )
                  : "No author found"}
              </p>
            </li>
          );
        })}
      </ul>
    </div>
  );
};

export default SearchWindow;
