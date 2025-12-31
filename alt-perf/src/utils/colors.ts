/**
 * CLI color utilities for alt-perf
 */

// ANSI color codes
const COLORS = {
  reset: "\x1b[0m",
  bold: "\x1b[1m",
  dim: "\x1b[2m",
  italic: "\x1b[3m",
  underline: "\x1b[4m",

  // Foreground colors
  black: "\x1b[30m",
  red: "\x1b[31m",
  green: "\x1b[32m",
  yellow: "\x1b[33m",
  blue: "\x1b[34m",
  magenta: "\x1b[35m",
  cyan: "\x1b[36m",
  white: "\x1b[37m",

  // Bright colors
  brightRed: "\x1b[91m",
  brightGreen: "\x1b[92m",
  brightYellow: "\x1b[93m",
  brightBlue: "\x1b[94m",
  brightMagenta: "\x1b[95m",
  brightCyan: "\x1b[96m",
  brightWhite: "\x1b[97m",

  // Background colors
  bgRed: "\x1b[41m",
  bgGreen: "\x1b[42m",
  bgYellow: "\x1b[43m",
  bgBlue: "\x1b[44m",
};

// Check if colors are supported
function supportsColor(): boolean {
  // Respect NO_COLOR environment variable
  if (Deno.env.get("NO_COLOR") !== undefined) {
    return false;
  }
  // Check for FORCE_COLOR
  if (Deno.env.get("FORCE_COLOR") !== undefined) {
    return true;
  }
  // Check if running in a TTY
  try {
    return Deno.stdout.isTerminal();
  } catch {
    return false;
  }
}

const colorEnabled = supportsColor();

// Apply color if supported
function applyColor(text: string, color: string): string {
  if (!colorEnabled) return text;
  return `${color}${text}${COLORS.reset}`;
}

// Color functions
export function bold(text: string): string {
  return applyColor(text, COLORS.bold);
}

export function dim(text: string): string {
  return applyColor(text, COLORS.dim);
}

export function italic(text: string): string {
  return applyColor(text, COLORS.italic);
}

export function underline(text: string): string {
  return applyColor(text, COLORS.underline);
}

export function red(text: string): string {
  return applyColor(text, COLORS.red);
}

export function green(text: string): string {
  return applyColor(text, COLORS.green);
}

export function yellow(text: string): string {
  return applyColor(text, COLORS.yellow);
}

export function blue(text: string): string {
  return applyColor(text, COLORS.blue);
}

export function magenta(text: string): string {
  return applyColor(text, COLORS.magenta);
}

export function cyan(text: string): string {
  return applyColor(text, COLORS.cyan);
}

export function white(text: string): string {
  return applyColor(text, COLORS.white);
}

export function brightRed(text: string): string {
  return applyColor(text, COLORS.brightRed);
}

export function brightGreen(text: string): string {
  return applyColor(text, COLORS.brightGreen);
}

export function brightYellow(text: string): string {
  return applyColor(text, COLORS.brightYellow);
}

export function brightBlue(text: string): string {
  return applyColor(text, COLORS.brightBlue);
}

// Rating-based color
export function ratingColor(
  rating: "good" | "needs-improvement" | "poor"
): (text: string) => string {
  switch (rating) {
    case "good":
      return green;
    case "needs-improvement":
      return yellow;
    case "poor":
      return red;
    default:
      return dim;
  }
}

// Score-based color
export function scoreColor(score: number): (text: string) => string {
  if (score >= 90) return green;
  if (score >= 50) return yellow;
  return red;
}

// Status indicators
export const SYMBOLS = {
  pass: colorEnabled ? green("✓") : "[PASS]",
  fail: colorEnabled ? red("✗") : "[FAIL]",
  warn: colorEnabled ? yellow("⚠") : "[WARN]",
  info: colorEnabled ? blue("ℹ") : "[INFO]",
  arrow: colorEnabled ? "→" : "->",
  bullet: colorEnabled ? "•" : "*",
  line: colorEnabled ? "─" : "-",
};

// Create horizontal line
export function horizontalLine(length: number = 80): string {
  return dim(SYMBOLS.line.repeat(length));
}
