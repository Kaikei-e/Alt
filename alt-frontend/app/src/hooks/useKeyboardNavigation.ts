import { useEffect } from 'react';

interface KeyboardNavigationOptions {
  onArrowUp: () => void;
  onArrowDown: () => void;
  onEnter: () => void;
  onSpace: () => void;
  element: HTMLElement | null;
}

export function useKeyboardNavigation({
  onArrowUp,
  onArrowDown,
  onEnter,
  onSpace,
  element
}: KeyboardNavigationOptions) {
  useEffect(() => {
    if (!element) return;

    const handleKeyDown = (event: KeyboardEvent) => {
      switch (event.key) {
        case 'ArrowUp':
          event.preventDefault();
          onArrowUp();
          break;
        case 'ArrowDown':
          event.preventDefault();
          onArrowDown();
          break;
        case 'Enter':
          event.preventDefault();
          onEnter();
          break;
        case ' ':
          event.preventDefault();
          onSpace();
          break;
      }
    };

    element.addEventListener('keydown', handleKeyDown);
    return () => element.removeEventListener('keydown', handleKeyDown);
  }, [element, onArrowUp, onArrowDown, onEnter, onSpace]);
}