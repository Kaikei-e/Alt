import useSWR from "swr";
import type {
  ScrapingDomain,
  UpdateScrapingDomainRequest,
} from "@/schema/scrapingDomain";
import { apiClient } from "./index";

const BASE_PATH = "/v1/admin/scraping-domains";

/**
 * Fetcher function for SWR
 */
async function fetcher<T>(url: string): Promise<T> {
  return apiClient.get<T>(url);
}

/**
 * Hook to fetch list of scraping domains
 */
export function useScrapingDomains(offset = 0, limit = 20) {
  const { data, error, isLoading, mutate } = useSWR<ScrapingDomain[]>(
    `${BASE_PATH}?offset=${offset}&limit=${limit}`,
    fetcher,
    {
      revalidateOnFocus: false,
      revalidateOnReconnect: true,
    },
  );

  return {
    domains: data ?? [],
    isLoading,
    error,
    mutate,
  };
}

/**
 * Hook to fetch a single scraping domain by ID
 */
export function useScrapingDomain(id: string | null) {
  const { data, error, isLoading, mutate } = useSWR<ScrapingDomain>(
    id ? `${BASE_PATH}/${id}` : null,
    fetcher,
    {
      revalidateOnFocus: false,
    },
  );

  return {
    domain: data,
    isLoading,
    error,
    mutate,
  };
}

/**
 * Update scraping domain policy
 */
export async function updateScrapingDomain(
  id: string,
  data: UpdateScrapingDomainRequest,
): Promise<void> {
  await apiClient.patch(`${BASE_PATH}/${id}`, data as Record<string, unknown>);
}

/**
 * Refresh robots.txt for a scraping domain
 */
export async function refreshRobotsTxt(id: string): Promise<void> {
  await apiClient.post(`${BASE_PATH}/${id}/refresh-robots`, {});
}
