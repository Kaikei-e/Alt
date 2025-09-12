// REPORT.md恒久対応: /desktop/articles/search ルート実装
// React #418エラー解決のため、RSCルートを追加

export const dynamic = "force-dynamic";
export const fetchCache = "force-no-store";
export const revalidate = 0;

import { Box, Heading, Text } from "@chakra-ui/react";

export default function DesktopArticlesSearchPage() {
  return (
    <Box p={8}>
      <Heading size="lg" mb={4}>
        Article Search
      </Heading>
      <Text color="gray.600">Desktop article search page implementation</Text>
    </Box>
  );
}
