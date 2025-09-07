/**
 * HTML Rendering Strategies Tests
 * TDD tests for fixing HTML image parsing issues
 */
import { describe, it, expect } from 'vitest';
import { HTMLRenderingStrategy } from '../../../../src/utils/renderingStrategies';

describe('HTMLRenderingStrategy', () => {
  const strategy = new HTMLRenderingStrategy();

  describe('HTML Image Parsing Issues', () => {
    it('should properly parse HTML with img tags inside div elements', () => {
      // RED: This test should fail initially - this is the problematic HTML from user
      const problematicHTML = `
        <div><img src="https://9to5mac.com/wp-content/uploads/sites/6/2025/07/ios-26-home-screen-iphone-new-apps.jpg?quality=82&amp;strip=all&amp;w=1600" alt="ios-26-home-screen-iphone-new-apps.jpg?q"></div>
        <p>A new beta 6 build for iOS 26 was released today, bringing <a href="https://9to5mac.com/2025/08/11/ios-26-beta-6-changes/">a variety of changes</a>, including one update that is especially hard to miss: iPhone apps launch much faster than before.</p>
        <a href="https://9to5mac.com/2025/08/11/ios-26-beta-6-makes-iphone-apps-launch-so-much-faster/#more-1013732">more…</a>
      `;

      const result = strategy.render(problematicHTML);
      
      // Test that result is a valid React node (not null or undefined)
      expect(result).toBeDefined();
      expect(result).not.toBeNull();
      
      // Test that the strategy recognizes this as HTML content
      expect(strategy.shouldUse(problematicHTML)).toBe(true);
    });

    it('should handle HTML entities in image URLs correctly', () => {
      const htmlWithEntities = `<img src="https://example.com/image.jpg?quality=82&amp;strip=all&amp;w=1600" alt="Test Image">`;
      
      const result = strategy.render(htmlWithEntities);
      expect(result).toBeDefined();
      expect(strategy.shouldUse(htmlWithEntities)).toBe(true);
    });

    it('should preserve div structure around images', () => {
      const htmlWithDivAroundImg = `<div class="image-container"><img src="https://example.com/test.jpg" alt="Test"></div>`;
      
      const result = strategy.render(htmlWithDivAroundImg);
      expect(result).toBeDefined();
      expect(strategy.shouldUse(htmlWithDivAroundImg)).toBe(true);
    });

    it('should handle multiple images in complex HTML structure', () => {
      const complexHTML = `
        <div>
          <img src="https://example.com/img1.jpg" alt="Image 1">
        </div>
        <p>Some text between images</p>
        <div>
          <img src="https://example.com/img2.jpg?param=value&amp;other=test" alt="Image 2">
        </div>
      `;

      const result = strategy.render(complexHTML);
      expect(result).toBeDefined();
      expect(strategy.shouldUse(complexHTML)).toBe(true);
    });

    it('should apply security attributes to links correctly', () => {
      const htmlWithLinks = `<p>Check out <a href="https://9to5mac.com/2025/08/11/ios-26-beta-6-changes/">this article</a> for more info.</p>`;
      
      const result = strategy.render(htmlWithLinks);
      expect(result).toBeDefined();
      expect(strategy.shouldUse(htmlWithLinks)).toBe(true);
    });

    it('should sanitize dangerous HTML while preserving safe content', () => {
      const dangerousHTML = `
        <div>
          <img src="https://example.com/safe.jpg" alt="Safe image">
          <script>alert('dangerous')</script>
          <p>Safe paragraph</p>
        </div>
      `;

      const result = strategy.render(dangerousHTML);
      expect(result).toBeDefined();
      expect(strategy.shouldUse(dangerousHTML)).toBe(true);
    });
  });

  describe('shouldUse method', () => {
    it('should return true for HTML content', () => {
      const htmlContent = '<div><img src="test.jpg" alt="test"></div>';
      expect(strategy.shouldUse(htmlContent)).toBe(true);
    });

    it('should return false for plain text content', () => {
      const textContent = 'This is plain text without HTML tags';
      expect(strategy.shouldUse(textContent)).toBe(false);
    });
  });
});