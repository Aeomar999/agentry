// A count of tokens as people read it: 950, 12.4k, 3.1M. Matches the
// server's coordinator.Tokens, which writes the budget question.
export function tokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1e6).toFixed(1).replace(/\.0$/, "")}M`;
  if (n >= 1_000) return `${(n / 1e3).toFixed(1).replace(/\.0$/, "")}k`;
  return String(n);
}

// "2M", "500k" or "300000" as a number of tokens, or NaN.
export function parseTokens(text: string): number {
  const match = text
    .trim()
    .toLowerCase()
    .replace(/,/g, "")
    .match(/^(\d+(?:\.\d+)?)\s*([km]?)$/);
  if (!match) return NaN;
  return Math.round(Number(match[1]) * (match[2] === "m" ? 1e6 : match[2] === "k" ? 1e3 : 1));
}
