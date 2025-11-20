import { Suspense } from "react";
import dynamic from "next/dynamic";

// Client component is loaded dynamically to reduce initial bundle
// Note: ssr: false is not allowed in Server Components, so we import directly
const SearchFeedsClient = dynamic(
  () => import("@/components/mobile/search/SearchFeedsClient"),
  {
    loading: () => (
      <div
        style={{
          minHeight: "100dvh",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        <div>Loading...</div>
      </div>
    ),
  }
);

export default function SearchFeedsPage() {
  return (
    <Suspense fallback={<div>Loading...</div>}>
      <SearchFeedsClient />
    </Suspense>
  );
}
