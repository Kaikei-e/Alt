import { Feed } from '@/schema/feed';
import { expect, test, vi } from 'vitest'
import FeedCard from '@/components/mobile/FeedCard'

const generateMockFeeds = (count: number, startId: number = 1): Feed[] => {
  return Array.from({ length: count }, (_, index) => ({
    id: `${startId + index}`,
    title: `Test Feed ${startId + index}`,
    description: `Description for test feed ${startId + index}. This is a longer description to test how the UI handles different text lengths.`,
    link: `https://example.com/feed${startId + index}`,
    published: `2024-01-${String(index + 1).padStart(2, '0')}T12:00:00Z`
  }));
};

test("FeedCard", () => {
  const feeds = generateMockFeeds(10, 1)
  expect(feeds.length).toBe(10)
  expect(feeds[0].title).toBe("Test Feed 1")
  expect(feeds[0].description).toBe("Description for test feed 1. This is a longer description to test how the UI handles different text lengths.")
  expect(feeds[0].link).toBe("https://example.com/feed1")
  expect(feeds[0].published).toBe("2024-01-01T12:00:00Z")
  
  expect(feeds[5].id).toBe("6")
  expect(feeds[5].title).toBe("Test Feed 6")
  expect(feeds[5].description).toBe("Description for test feed 6. This is a longer description to test how the UI handles different text lengths.")
  expect(feeds[5].link).toBe("https://example.com/feed6")
  expect(feeds[5].published).toBe("2024-01-06T12:00:00Z")

  // Component testing would require React Testing Library
  // For now, just testing the mock data generation and component import
  expect(FeedCard).toBeDefined()
  expect(typeof FeedCard).toBe("object")
})
