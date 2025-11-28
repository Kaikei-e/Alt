import type { Article } from "./article";

export type MorningUpdate = {
  group_id: string;
  primary_article: Article | null;
  duplicates: Article[];
};
