// What was pasted or typed into the sign-in page: the link `agentry serve`
// printed (its token=), a QR code's link (its signin=), the bare token, or a
// one-time code, typed with or without its dash.

const codeLength = 8; // as internal/web/signin.go makes them; the token is far longer

export function readSignIn(typed: string): { code: string } | { token: string } {
  const text = typed.trim();
  if (/^https?:\/\//i.test(text)) {
    const query = new URL(text).searchParams;
    const token = query.get("token");
    const code = query.get("signin");
    if (token) return { token };
    if (code) return { code };
    throw new Error("That link has no token in it: paste the link `agentry serve` printed.");
  }
  return text.replace(/[\s-]/g, "").length <= codeLength ? { code: text } : { token: text };
}
