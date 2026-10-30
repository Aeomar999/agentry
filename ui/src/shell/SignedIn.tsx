import { ReactNode, useState, useSyncExternalStore } from "react";
import { isSignedOut, signinCodeFromLink, watchSignedOut } from "@/api/client";
import { SignInPage } from "@/shell/signin/SignInPage";

// SignedIn shows the sign-in page in place of the app when the server says
// the page isn't signed in, or when the page opened with a one-time code to
// use. Signing in starts the app afresh, so nothing on it keeps what failed.
export function SignedIn({ children }: { children: ReactNode }) {
  const signedOut = useSyncExternalStore(watchSignedOut, isSignedOut);
  const [code, setCode] = useState(signinCodeFromLink);
  if (signedOut || code) return <SignInPage code={code} onSignedIn={() => setCode("")} />;
  return children;
}
