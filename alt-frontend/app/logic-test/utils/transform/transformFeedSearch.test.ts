import { transformFeedSearchResult } from "@/lib/utils/transformFeedSearchResult";
import { describe, expect, it } from "vitest";

describe("transformFeedSearchResult", () => {
  const mockDataForSuccessfullSearchResult = {
    results: [
      {
        title: "Artificial Intelligence",
        description: "Artificial Intelligence is the future",
        link: "https://www.example.com",
        published: "2021-01-01",
        authors: [{ name: "John Doe" }],
      },
      {
        title: "Artificial Intelligence and Machine Learning",
        description:
          "Artificial Intelligence and Machine Learning are the future",
        link: "https://www.example.com",
        published: "2021-01-01",
        authors: [{ name: "Jane Doe" }],
      },
    ],
    error: null,
  };

  const mockDataForEmptySearchResult = {
    results: [],
    error: null,
  };

  const mockDataForErrorSearchResult = {
    results: [],
    error: "Error",
  };

  const mockDataForSuccessfullSearch = [
    {
      title: "Artificial Intelligence",
      description: "Artificial Intelligence is the future",
      link: "https://www.example.com",
      published: "2021-01-01",
      authors: [{ name: "John Doe" }],
    },
    {
      title: "Artificial Intelligence and Machine Learning",
      description:
        "Artificial Intelligence and Machine Learning are the future",
      link: "https://www.example.com",
      published: "2021-01-01",
      authors: [{ name: "Jane Doe" }],
    },
  ];

  it("should transform the feed search result", () => {
    const transformedFeedSearchResult = transformFeedSearchResult(
      mockDataForSuccessfullSearchResult,
    );
    expect(transformedFeedSearchResult).toEqual(mockDataForSuccessfullSearch);
  });

  it("should return an empty array if the feed search result is empty", () => {
    const transformedFeedSearchResult = transformFeedSearchResult({
      results: [],
      error: null,
    });
    expect(transformedFeedSearchResult).toEqual([]);
  });

  it("should return an empty array if the feed search result is null", () => {
    const transformedFeedSearchResult = transformFeedSearchResult(
      mockDataForErrorSearchResult,
    );
    expect(transformedFeedSearchResult).toEqual([]);
  });

  it("should return an empty array if the feed search result is undefined", () => {
    const transformedFeedSearchResult = transformFeedSearchResult(
      mockDataForEmptySearchResult,
    );
    expect(transformedFeedSearchResult).toEqual([]);
  });
});
