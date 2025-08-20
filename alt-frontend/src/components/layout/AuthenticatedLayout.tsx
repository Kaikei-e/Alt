"use client";

import { ReactNode, useState } from 'react';
import {
  Box,
  Flex,
  VStack,
  HStack,
  Text,
  Button,
  IconButton,
  CloseButton,
  Drawer,
  Portal,
  Separator,
  useDisclosure,
} from '@chakra-ui/react';
import {
  Menu,
  Home,
  BookOpen,
  Search,
  Settings,
  LogOut,
  User,
  Bell,
  Activity,
  Clock
} from 'lucide-react';
import { useAuth } from '@/contexts/auth-context';
import { UserProfile } from '@/components/auth/UserProfile';

interface AuthenticatedLayoutProps {
  children: ReactNode;
  showHeader?: boolean;
  showSidebar?: boolean;
  showFooter?: boolean;
  maxWidth?: string;
}

interface NavigationItem {
  id: string;
  label: string;
  icon: React.ReactNode;
  href: string;
  requireAuth?: boolean;
  requiredRole?: string;
  badge?: string | number;
}

const navigationItems: NavigationItem[] = [
  {
    id: 'home',
    label: 'ホーム',
    icon: <Home size={20} />,
    href: '/',
  },
  {
    id: 'feeds',
    label: 'フィード',
    icon: <BookOpen size={20} />,
    href: '/feeds',
    requireAuth: true,
  },
  {
    id: 'search',
    label: '検索',
    icon: <Search size={20} />,
    href: '/search',
  },
  {
    id: 'notifications',
    label: '通知',
    icon: <Bell size={20} />,
    href: '/notifications',
    requireAuth: true,
    badge: 3,
  },
  {
    id: 'activity',
    label: '活動',
    icon: <Activity size={20} />,
    href: '/activity',
    requireAuth: true,
  },
  {
    id: 'settings',
    label: '設定',
    icon: <Settings size={20} />,
    href: '/settings',
    requireAuth: true,
  },
];

export function AuthenticatedLayout({
  children,
  showHeader = true,
  showSidebar = true,
  showFooter = true,
  maxWidth = "1200px",
}: AuthenticatedLayoutProps) {
  const { user, isAuthenticated, logout, lastActivity, sessionTimeout } = useAuth();
  const { open, onOpen, onClose } = useDisclosure();
  const [activeNavItem, setActiveNavItem] = useState<string>('home');

  const handleNavItemClick = (item: NavigationItem) => {
    if (item.requireAuth && !isAuthenticated) {
      // Redirect to login or show login modal
      return;
    }

    if (item.requiredRole && user?.role !== item.requiredRole) {
      // Show insufficient permissions message
      return;
    }

    setActiveNavItem(item.id);
    onClose(); // Close mobile drawer
    // Navigate to href (implement navigation logic)
    window.location.href = item.href;
  };

  const handleLogout = async () => {
    try {
      await logout();
      window.location.href = '/';
    } catch (error) {
      console.error('Logout error:', error);
    }
  };

  const getFilteredNavigationItems = () => {
    return navigationItems.filter(item => {
      if (item.requireAuth && !isAuthenticated) return false;
      if (item.requiredRole && user?.role !== item.requiredRole) return false;
      return true;
    });
  };

  const renderNavigationItem = (item: NavigationItem, isMobile = false) => (
    <Button
      key={item.id}
      variant={activeNavItem === item.id ? "solid" : "ghost"}
      w={isMobile ? "full" : "auto"}
      bg={activeNavItem === item.id ? "var(--alt-primary)" : "transparent"}
      color={activeNavItem === item.id ? "white" : "var(--text-primary)"}
      fontFamily="body"
      justifyContent={isMobile ? "flex-start" : "center"}
      onClick={() => handleNavItemClick(item)}
      _hover={{
        bg: activeNavItem === item.id ? "var(--alt-primary)" : "var(--alt-glass)",
        transform: "translateY(-1px)",
      }}
      position="relative"
    >
      <Flex align="center" gap={isMobile ? 3 : 2}>
        {item.icon}
        {isMobile && (
          <Text fontSize="sm" fontWeight="medium">
            {item.label}
          </Text>
        )}
        {item.badge && (
          <Box
            position="absolute"
            top="-2px"
            right="-2px"
            bg="semantic.error"
            color="white"
            borderRadius="full"
            minW="18px"
            h="18px"
            display="flex"
            alignItems="center"
            justifyContent="center"
            fontSize="xs"
            fontWeight="bold"
          >
            {item.badge}
          </Box>
        )}
      </Flex>
    </Button>
  );

  const SessionIndicator = () => {
    if (!isAuthenticated || !lastActivity || !sessionTimeout) return null;

    const now = new Date();
    const minutesSinceLastActivity = Math.floor((now.getTime() - lastActivity.getTime()) / (1000 * 60));
    const timeRemaining = sessionTimeout - minutesSinceLastActivity;
    const showWarning = timeRemaining <= 5 && timeRemaining > 0;

    return (
      <Flex align="center" gap={2}>
        <Clock size={12} />
        <Text
          fontSize="xs"
          color={showWarning ? "semantic.warning" : "var(--text-muted)"}
          fontFamily="body"
        >
          {timeRemaining}分
        </Text>
      </Flex>
    );
  };

  return (
    <Box minH="100vh" bg="var(--background)">
      {/* Header */}
      {showHeader && (
        <Box
          as="header"
          bg="var(--alt-glass)"
          backdropFilter="blur(12px)"
          border="1px solid"
          borderColor="var(--alt-glass-border)"
          position="sticky"
          top={0}
          zIndex={1000}
          w="full"
        >
          <Box maxW={maxWidth} mx="auto" px={4}>
            <Flex align="center" justify="space-between" h="64px">
              {/* Logo / Brand */}
              <Flex align="center" gap={3}>
                <IconButton
                  aria-label="メニューを開く"
                  variant="ghost"
                  display={{ base: "flex", md: "none" }}
                  onClick={onOpen}
                >
                  <Menu size={20} />
                </IconButton>
                <Text
                  fontSize="xl"
                  fontWeight="bold"
                  fontFamily="heading"
                  color="var(--alt-primary)"
                >
                  Alt
                </Text>
              </Flex>

              {/* Desktop Navigation */}
              <HStack gap={2} display={{ base: "none", md: "flex" }}>
                {getFilteredNavigationItems().map(item => renderNavigationItem(item))}
              </HStack>

              {/* User Menu */}
              <Flex align="center" gap={3}>
                <SessionIndicator />

                {isAuthenticated ? (
                  <HStack gap={2}>
                    <Box
                      w="2rem"
                      h="2rem"
                      bg="var(--alt-primary)"
                      color="white"
                      borderRadius="full"
                      display="flex"
                      alignItems="center"
                      justifyContent="center"
                      fontFamily="heading"
                      fontWeight="semibold"
                      fontSize="sm"
                    >
                      {user?.name ? user.name[0].toUpperCase() : user?.email?.[0].toUpperCase() || 'U'}
                    </Box>

                    <IconButton
                      aria-label="ログアウト"
                      variant="ghost"
                      size="sm"
                      color="var(--text-muted)"
                      onClick={handleLogout}
                      _hover={{
                        color: "semantic.error",
                        bg: "var(--alt-glass)",
                      }}
                    >
                      <LogOut size={16} />
                    </IconButton>
                  </HStack>
                ) : (
                  <Button
                    size="sm"
                    bg="var(--alt-primary)"
                    color="white"
                    fontFamily="body"
                    _hover={{
                      bg: "var(--alt-primary)",
                      transform: "translateY(-1px)",
                    }}
                    onClick={() => {
                      const currentUrl = typeof window !== 'undefined' ? window.location.href : '/';
                      window.location.href = `/auth/login?return_to=${encodeURIComponent(currentUrl)}`
                    }}
                  >
                    ログイン
                  </Button>
                )}
              </Flex>
            </Flex>
          </Box>
        </Box>
      )}

      {/* Mobile Drawer */}
      <Drawer.Root open={open} onOpenChange={({ open }) => { if (!open) onClose(); }} placement="start">
        <Portal>
          <Drawer.Backdrop />
          <Drawer.Positioner>
            <Drawer.Content
              bg="var(--alt-glass)"
              backdropFilter="blur(12px)"
              border="1px solid"
              borderColor="var(--alt-glass-border)"
            >
              <Drawer.CloseTrigger asChild>
                <CloseButton color="var(--text-primary)" />
              </Drawer.CloseTrigger>
              <Drawer.Header>
                <Text
                  fontSize="lg"
                  fontWeight="bold"
                  fontFamily="heading"
                  color="var(--alt-primary)"
                >
                  Alt
                </Text>
              </Drawer.Header>

              <Drawer.Body>
                <VStack gap={4} align="stretch">
                  {/* User Profile in Mobile */}
                  {isAuthenticated && user ? (
                    <Box>
                      <UserProfile />
                      <Separator my={4} borderColor="var(--alt-glass-border)" />
                    </Box>
                  ) : (
                    <Box>
                      <Button
                        w="full"
                        bg="var(--alt-primary)"
                        color="white"
                        fontFamily="body"
                        onClick={() => {
                          onClose();
                          const currentUrl = typeof window !== 'undefined' ? window.location.href : '/';
                          window.location.href = `/auth/login?return_to=${encodeURIComponent(currentUrl)}`
                        }}
                        _hover={{
                          bg: "var(--alt-primary)",
                          transform: "translateY(-1px)",
                        }}
                      >
                        ログイン
                      </Button>
                      <Separator my={4} borderColor="var(--alt-glass-border)" />
                    </Box>
                  )}

                  {/* Mobile Navigation */}
                  <VStack gap={2} align="stretch">
                    {getFilteredNavigationItems().map(item => renderNavigationItem(item, true))}
                  </VStack>
                </VStack>
              </Drawer.Body>
            </Drawer.Content>
          </Drawer.Positioner>
        </Portal>
      </Drawer.Root>

      {/* Main Content */}
      <Box
        as="main"
        flex={1}
        maxW={maxWidth}
        mx="auto"
        px={4}
        py={6}
        w="full"
      >
        {children}
      </Box>

      {/* Footer */}
      {showFooter && (
        <Box
          as="footer"
          bg="var(--alt-glass)"
          backdropFilter="blur(12px)"
          border="1px solid"
          borderColor="var(--alt-glass-border)"
          mt="auto"
        >
          <Box maxW={maxWidth} mx="auto" px={4} py={8}>
            <VStack gap={4} textAlign="center">
              <Text
                fontSize="lg"
                fontWeight="bold"
                fontFamily="heading"
                color="var(--alt-primary)"
              >
                Alt
              </Text>

              <Text
                fontSize="sm"
                color="var(--text-muted)"
                fontFamily="body"
              >
                モバイルファーストRSSリーダー
              </Text>

              <HStack gap={6} wrap="wrap" justify="center">
                <Button variant="ghost" size="sm" color="var(--text-muted)">
                  利用規約
                </Button>
                <Button variant="ghost" size="sm" color="var(--text-muted)">
                  プライバシーポリシー
                </Button>
                <Button variant="ghost" size="sm" color="var(--text-muted)">
                  お問い合わせ
                </Button>
              </HStack>

              <Text
                fontSize="xs"
                color="var(--text-muted)"
                fontFamily="body"
              >
                © 2025 Alt. All rights reserved.
              </Text>
            </VStack>
          </Box>
        </Box>
      )}
    </Box>
  );
}
