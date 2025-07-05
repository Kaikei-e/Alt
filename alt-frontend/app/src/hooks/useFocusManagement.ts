import { useState, useCallback } from 'react';

export function useFocusManagement() {
  const [focused, setFocused] = useState(false);

  const handleFocus = useCallback(() => {
    setFocused(true);
  }, []);

  const handleBlur = useCallback(() => {
    setFocused(false);
  }, []);

  return {
    focused,
    handleFocus,
    handleBlur
  };
}