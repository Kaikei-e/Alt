## Alt Frontend

This is the frontend for the Alt project - a mobile-first RSS reader built with a microservice architecture stack.

## Features

### Core Functionality

- **Feed Management**: View and manage RSS feeds with real-time updates
- **Read Status Tracking**: Mark feeds as read and view read feed history
- **Search Capabilities**: Search through feeds and articles
- **Mobile-First Design**: Optimized for mobile devices with responsive design

### Recently Added

- **Viewed Feeds Page** (`/mobile/feeds/viewed`): Dedicated page for viewing previously read articles
- **Enhanced Navigation**: FloatingMenu with active state indicators
- **Optimized Performance**: Cursor-based pagination with prefetching capabilities

## Pages Overview

- `/` - Home page
- `/mobile/feeds` - Main feeds listing with infinite scroll
- `/mobile/feeds/viewed` - Read feeds archive with cursor pagination
- `/mobile/feeds/register` - RSS feed registration
- `/mobile/feeds/search` - Feed search functionality
- `/mobile/articles/search` - Article search functionality
- `/mobile/feeds/stats` - Feed statistics dashboard

## Getting Started

1. Clone the repository
2. Run `pnpm install` to install the dependencies
3. Run `pnpm run dev` to start the development server

## Architecture

Built following TDD (Test-Driven Development) principles with:

- **TypeScript** for type safety
- **Next.js** (Pages Router) for server-side rendering
- **Chakra UI** for component library
- **Vitest** for unit testing
- **Playwright** for E2E testing

## API Integration

The frontend integrates with the alt-backend microservice:

- **Endpoint**: `/mobile/feeds/viewed`
- **API**: `getReadFeedsWithCursor` for cursor-based pagination
- **Implementation**: TDD + Clean Architecture patterns
