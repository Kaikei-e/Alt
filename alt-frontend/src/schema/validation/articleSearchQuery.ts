import * as v from "valibot";

export type ArticleSearchQuery = {
  query: string;
};

export const articleSearchQuerySchema = v.object({
  query: v.pipe(
    v.string("Please enter a search query"),
    v.trim(),
    v.nonEmpty("Please enter a search query"),
    v.minLength(2, "Search query must be at least 2 characters"),
    v.maxLength(100, "Search query must be at most 100 characters"),
  ),
});
