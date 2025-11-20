import { Suspense } from "react";
import dynamic from "next/dynamic";

// Load SwipeFeedScreen dynamically to reduce initial bundle
// Note: ssr: false is not allowed in Server Components, so we remove it
// The component itself is already a client component, so it will be client-side only
const SwipeFeedScreen = dynamic(
  () => import("@/components/mobile/feeds/swipe/SwipeFeedScreen"),
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

export default function SwipeFeedsPage() {
  return (
    <Suspense fallback={<div>Loading...</div>}>
      <SwipeFeedScreen />
    </Suspense>
  );
}
