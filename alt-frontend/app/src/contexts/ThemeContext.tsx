"use client";

import { createContext } from 'react'
import type { ThemeContextType } from '../types/theme'

export const ThemeContext = createContext<ThemeContextType | null>(null)