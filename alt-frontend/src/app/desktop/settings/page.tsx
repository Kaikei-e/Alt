// REPORT.md恒久対応: /desktop/settings ルート実装
// React #418エラー解決のため、RSCルートを追加

import { Box, Heading, Text } from '@chakra-ui/react'

// グローバル設定により動的レンダリングは layout.tsx で自動適用

export default function DesktopSettingsPage() {
  return (
    <Box p={8}>
      <Heading size="lg" mb={4}>
        Settings
      </Heading>
      <Text color="gray.600">
        Desktop settings page implementation
      </Text>
    </Box>
  )
}