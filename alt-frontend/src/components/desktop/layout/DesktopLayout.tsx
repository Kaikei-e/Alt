import { Box, Flex } from "@chakra-ui/react";
import type React from "react";
import { ThemeToggle } from "@/components/ThemeToggle";
import { DesktopSidebar } from "./DesktopSidebar";

interface DesktopLayoutProps {
  children: React.ReactNode;
  showSidebar?: boolean;
  showRightPanel?: boolean;
  rightPanel?: React.ReactNode;
  sidebarProps?: {
    navItems: Array<{
      id: number;
      label: string;
      icon?: React.ComponentType<{ size?: number }>;
      iconName?: string; // Support icon names for Server/Client boundary
      href: string;
      active?: boolean;
    }>;
    logoText?: string;
    logoSubtext?: string;
  };
}

export const DesktopLayout: React.FC<DesktopLayoutProps> = ({
  children,
  showSidebar = true,
  showRightPanel = false,
  rightPanel,
  sidebarProps,
}) => {
  return (
    <Box minH="100vh" bg="var(--app-bg)" position="relative" data-testid="desktop-shell">
      {/* Theme Toggle - only show when no right panel */}
      {!showRightPanel && (
        <Box position="fixed" top={4} right={4} zIndex={1000}>
          <ThemeToggle size="md" />
        </Box>
      )}

      <Flex minH="100vh">
        {/* Left Sidebar */}
        {showSidebar && sidebarProps && (
          <Box
            w="250px"
            minH="100vh"
            p={6}
            className="glass"
            borderRadius="0"
            borderRight="1px solid var(--surface-border)"
            position="fixed"
            left={0}
            top={0}
            bg="var(--surface-bg)"
            backdropFilter="blur(var(--surface-blur))"
            data-testid="desktop-navigation"
          >
            <DesktopSidebar
              navItems={sidebarProps.navItems}
              logoText={sidebarProps.logoText || "Alt Dashboard"}
              logoSubtext={sidebarProps.logoSubtext || "RSS Management Hub"}
              mode="navigation"
            />
          </Box>
        )}

        {/* Main Content Area */}
        <Box
          flex="1"
          ml={showSidebar ? "250px" : "0"}
          mr={showRightPanel ? "330px" : "0"}
          data-testid="main-content"
        >
          {children}
        </Box>

        {/* Right Panel */}
        {showRightPanel && rightPanel && (
          <Box
            w="330px"
            minH="100vh"
            p={6}
            className="glass"
            borderRadius="0"
            borderLeft="1px solid var(--surface-border)"
            position="fixed"
            right={0}
            top={0}
            bg="var(--surface-bg)"
            backdropFilter="blur(var(--surface-blur))"
            overflowY="auto"
            data-testid="right-panel"
          >
            {rightPanel}
          </Box>
        )}
      </Flex>
    </Box>
  );
};
