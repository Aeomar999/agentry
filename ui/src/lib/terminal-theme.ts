import type { ITheme } from "@xterm/xterm";

// The terminals' colours. The screen and text come from the page's tokens;
// the sixteen ANSI colours are VS Code's for a light screen, and xterm's
// own for a dark one, so programs that colour their output stay readable.

const LIGHT_ANSI: ITheme = {
  black: "#000000",
  red: "#cd3131",
  green: "#107c10",
  yellow: "#949800",
  blue: "#0451a5",
  magenta: "#bc05bc",
  cyan: "#0598bc",
  white: "#555555",
  brightBlack: "#666666",
  brightRed: "#cd3131",
  brightGreen: "#14ce14",
  brightYellow: "#b5ba00",
  brightBlue: "#0451a5",
  brightMagenta: "#bc05bc",
  brightCyan: "#0598bc",
  brightWhite: "#a5a5a5",
};

function token(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

export function terminalTheme(): ITheme {
  const light = document.documentElement.classList.contains("light");
  const screen: ITheme = {
    background: token("--color-terminal"),
    foreground: token("--color-ink-primary"),
    cursor: token("--color-ink-primary"),
    selectionBackground: token("--color-surface-3"),
  };
  return light ? { ...LIGHT_ANSI, ...screen } : screen;
}
