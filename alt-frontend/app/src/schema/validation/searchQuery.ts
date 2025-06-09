import * as v from "valibot";

export type SearchQuery = {
  query: string;
};

export const searchQuerySchema = v.object({
  query: v.pipe(
    v.string("Please enter a search query"),
    v.maxLength(100, "Search query must be at most 100 characters"),
    v.minLength(2, "Search query must be at least 2 characters"),
    v.regex(
      /^[a-zA-Z0-9\s\-_.']+$/,
      "Search query must contain only letters, numbers, spaces, and basic punctuation",
    ),
    v.nonEmpty("You must enter a search query"),
  ),
});
