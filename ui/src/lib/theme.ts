// The theme is the class on <html>: none for dark, "light" for light.
// The choice is the viewer's own, so it lives in this browser only.

export type Theme = "dark" | "light";

const key = "agentry-theme";

export function savedTheme(): Theme {
  try {
    return localStorage.getItem(key) === "light" ? "light" : "dark";
  } catch {
    return "dark";
  }
}

export function applyTheme(theme: Theme): void {
  document.documentElement.classList.toggle("light", theme === "light");
  try {
    localStorage.setItem(key, theme);
  } catch {
    // a private window: the theme lasts until the tab closes
  }
}
