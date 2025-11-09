// Backward compatibility re-export for legacy imports
export type { CursorResponse } from "@/schema/common";

export {
  ApiClientError,
  apiClient,
  articleApi,
  desktopApi,
  feedApi,
  recapApi,
  serverFetch,
} from "./lib/api/index";
