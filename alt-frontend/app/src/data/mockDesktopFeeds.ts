import { DesktopFeed } from '@/types/desktop-feed';

export const mockDesktopFeeds: DesktopFeed[] = [
  {
    id: '1',
    title: 'OpenAI Announces GPT-5 with Revolutionary Capabilities for Enterprise Users',
    description: 'The latest AI breakthrough promises to transform business automation and decision-making processes with unprecedented accuracy and efficiency.',
    link: 'https://techcrunch.com/2024/openai-gpt5-enterprise',
    published: '2024-01-15T10:30:00Z',
    metadata: {
      source: {
        id: 'techcrunch',
        name: 'TechCrunch',
        icon: '📰',
        reliability: 9.2,
        category: 'tech',
        unreadCount: 12,
        avgReadingTime: 4.5
      },
      readingTime: 5,
      engagement: {
        views: 245,
        comments: 12,
        likes: 89,
        bookmarks: 23
      },
      tags: ['AI', 'OpenAI', 'Enterprise', 'Technology'],
      relatedCount: 3,
      publishedAt: '2 hours ago',
      author: 'Sarah Johnson',
      summary: 'This breakthrough in AI technology promises to transform how businesses approach automation and decision-making processes...',
      priority: 'high',
      category: 'tech',
      difficulty: 'intermediate'
    },
    isRead: false,
    isFavorited: false,
    isBookmarked: false
  },
  {
    id: '2',
    title: 'The Future of Web Development: React 19 and Beyond',
    description: 'Exploring the latest features and improvements in React 19, including concurrent rendering and server components.',
    link: 'https://dev.to/react-19-future',
    published: '2024-01-15T09:15:00Z',
    metadata: {
      source: {
        id: 'devto',
        name: 'Dev.to',
        icon: '💻',
        reliability: 8.7,
        category: 'development',
        unreadCount: 6,
        avgReadingTime: 6.2
      },
      readingTime: 8,
      engagement: {
        views: 189,
        comments: 23,
        likes: 156,
        bookmarks: 34
      },
      tags: ['React', 'JavaScript', 'Web Development', 'Frontend'],
      relatedCount: 5,
      publishedAt: '3 hours ago',
      author: 'Alex Chen',
      summary: 'React 19 introduces game-changing features that will revolutionize how we build modern web applications...',
      priority: 'medium',
      category: 'development',
      difficulty: 'advanced'
    },
    isRead: false,
    isFavorited: true,
    isBookmarked: false
  },
  {
    id: '3',
    title: 'Sustainable Design Practices for Modern Digital Products',
    description: 'How to create environmentally conscious designs that reduce digital carbon footprints while maintaining excellent user experiences.',
    link: 'https://medium.com/sustainable-design-2024',
    published: '2024-01-15T08:45:00Z',
    metadata: {
      source: {
        id: 'medium',
        name: 'Medium',
        icon: '📝',
        reliability: 8.1,
        category: 'design',
        unreadCount: 15,
        avgReadingTime: 5.8
      },
      readingTime: 6,
      engagement: {
        views: 156,
        comments: 9,
        likes: 67,
        bookmarks: 18
      },
      tags: ['Design', 'Sustainability', 'UX', 'Environment'],
      relatedCount: 2,
      publishedAt: '4 hours ago',
      author: 'Maria Rodriguez',
      summary: 'Sustainable design is becoming increasingly important as digital products consume more energy...',
      priority: 'low',
      category: 'design',
      difficulty: 'beginner'
    },
    isRead: true,
    isFavorited: false,
    isBookmarked: true,
    readingProgress: 78
  },
  {
    id: '4',
    title: 'Building Scalable Microservices with Go and Kubernetes',
    description: 'A comprehensive guide to architecting distributed systems using Go microservices deployed on Kubernetes.',
    link: 'https://blog.golang.org/microservices-kubernetes',
    published: '2024-01-15T07:20:00Z',
    metadata: {
      source: {
        id: 'golang',
        name: 'Go Blog',
        icon: '🐹',
        reliability: 9.5,
        category: 'backend',
        unreadCount: 8,
        avgReadingTime: 7.3
      },
      readingTime: 12,
      engagement: {
        views: 312,
        comments: 18,
        likes: 143,
        bookmarks: 56
      },
      tags: ['Go', 'Microservices', 'Kubernetes', 'Backend', 'DevOps'],
      relatedCount: 7,
      publishedAt: '5 hours ago',
      author: 'John Doe',
      summary: 'Learn how to build production-ready microservices using Go\'s concurrent features and Kubernetes orchestration...',
      priority: 'high',
      category: 'backend',
      difficulty: 'advanced'
    },
    isRead: false,
    isFavorited: true,
    isBookmarked: false
  },
  {
    id: '5',
    title: 'CSS Grid Layout: Advanced Techniques for Complex Layouts',
    description: 'Master advanced CSS Grid techniques to create complex, responsive layouts with minimal code.',
    link: 'https://css-tricks.com/advanced-css-grid',
    published: '2024-01-15T06:10:00Z',
    metadata: {
      source: {
        id: 'csstricks',
        name: 'CSS-Tricks',
        icon: '🎨',
        reliability: 8.9,
        category: 'frontend',
        unreadCount: 4,
        avgReadingTime: 4.8
      },
      readingTime: 7,
      engagement: {
        views: 201,
        comments: 15,
        likes: 92,
        bookmarks: 28
      },
      tags: ['CSS', 'Grid', 'Layout', 'Frontend', 'Web Design'],
      relatedCount: 4,
      publishedAt: '6 hours ago',
      author: 'Chris Coyier',
      summary: 'Explore powerful CSS Grid techniques that go beyond basic layouts to create stunning responsive designs...',
      priority: 'medium',
      category: 'frontend',
      difficulty: 'intermediate'
    },
    isRead: false,
    isFavorited: false,
    isBookmarked: true
  }
];