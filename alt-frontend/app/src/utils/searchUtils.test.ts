import { describe, it, expect } from 'vitest';
import { searchFeeds, simpleSearch } from './searchUtils';
import { SanitizedFeed } from '@/schema/feed';

describe('Search Utils', () => {
  const mockFeeds: SanitizedFeed[] = [
    {
      id: '1',
      title: 'React Tutorial',
      description: 'Learn React basics',
      link: 'https://example.com/react',
      published: '2023-01-01',
    },
    {
      id: '2',
      title: 'Vue.js Guide',
      description: 'Vue.js development guide',
      link: 'https://example.com/vue',
      published: '2023-01-02',
    },
  ];

  describe('searchFeeds', () => {
    it('should return highlighted results without XSS vulnerabilities', () => {
      const maliciousFeed: SanitizedFeed = {
        id: '3',
        title: '<script>alert("xss")</script>Tutorial',
        description: 'Learn <img src=x onerror=alert(1)> basics',
        link: 'https://example.com/malicious',
        published: '2023-01-03',
      };

      const results = searchFeeds([maliciousFeed], 'tutorial basics');
      
      expect(results).toHaveLength(1);
      expect(results[0].highlightedTitle).toBeDefined();
      expect(results[0].highlightedDescription).toBeDefined();
      
      // XSS攻撃ベクトルが適切にエスケープされていることを確認
      expect(results[0].highlightedTitle).not.toContain('<script>');
      expect(results[0].highlightedTitle).not.toContain('</script>');
      expect(results[0].highlightedDescription).not.toContain('<img');
      expect(results[0].highlightedDescription).not.toContain('onerror=');
      
      // 危険なHTMLタグが適切にエスケープされていることを確認
      expect(results[0].highlightedTitle).toContain('&lt;script&gt;');
      expect(results[0].highlightedDescription).toContain('&lt;img');
      
      // ハイライトマークが適切に適用されていることを確認
      expect(results[0].highlightedTitle).toContain('<mark>');
      expect(results[0].highlightedDescription).toContain('<mark>');
    });

    it('should handle normal search without breaking functionality', () => {
      const results = searchFeeds(mockFeeds, 'react');
      
      expect(results).toHaveLength(1);
      expect(results[0].feed.title).toBe('React Tutorial');
      expect(results[0].highlightedTitle).toContain('<mark>React</mark>');
    });

    it('should handle multiple keywords', () => {
      const results = searchFeeds(mockFeeds, 'react tutorial');
      
      expect(results).toHaveLength(1);
      expect(results[0].highlightedTitle).toContain('<mark>React</mark>');
      expect(results[0].highlightedTitle).toContain('<mark>Tutorial</mark>');
    });

    it('should handle empty query', () => {
      const results = searchFeeds(mockFeeds, '');
      expect(results).toHaveLength(mockFeeds.length);
    });
  });

  describe('simpleSearch', () => {
    it('should filter feeds by query', () => {
      const results = simpleSearch(mockFeeds, 'react');
      expect(results).toHaveLength(1);
      expect(results[0].title).toBe('React Tutorial');
    });

    it('should handle empty query', () => {
      const results = simpleSearch(mockFeeds, '');
      expect(results).toHaveLength(mockFeeds.length);
    });
  });
});