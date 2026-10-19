// What was pasted or typed into the sign-in page: the link `agentry serve`
// printed (its token=), a QR code's link (its signin=), the bare token, or a
// one-time code, typed with or without its dash. A link may come with the
// words around it, such as the whole line the server printed.

const codeLength = 8; // as internal/web/signin.go makes them; the token is far longer

export function readSignIn(typed: string): { code: string } | { token: string } {
  const text = typed.trim();
  const link = text.match(/https?:\/\/\S+/i)?.[0];
  if (link) {
    const query = queryOf(link);
    const token = query.get("token");
    const code = query.get("signin");
    if (token) return { token };
    if (code) return { code };
    throw new Error("That link has no token or code in it: paste the link `agentry serve` printed.");
  }
  return text.replace(/[\s-]/g, "").length <= codeLength ? { code: text } : { token: text };
}

// queryOf is a link's query, or none when the link is broken, such as half
// of it pasted. (Not URL.canParse: older iPhones, where installed apps
// matter most, don't have it.)
function queryOf(link: string): URLSearchParams {
  try {
    return new URL(link).searchParams;
  } catch {
    return new URLSearchParams();
  }
}
