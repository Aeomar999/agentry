import { FormEvent, useEffect, useRef, useState } from "react";
import { Button } from "@/components/atoms/Button";
import { Input } from "@/components/atoms/Input";
import { LogoTile } from "@/components/atoms/LogoTile";
import { Banner } from "@/components/molecules/Banner";
import { Field } from "@/components/molecules/Field";
import { signIn } from "@/api/client";
import { readSignIn } from "@/lib/signin";

// The page a device without a valid token sees, such as the installed app,
// which opens at "/" without the printed link. One field takes a one-time
// code from a device that is signed in, the token, or the printed link.
// Opened from a QR code's link, it signs in with the code by itself.
export function SignInPage({ code, onSignedIn }: { code: string; onSignedIn: () => void }) {
  const [typed, setTyped] = useState(code);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(text: string) {
    setBusy(true);
    setError("");
    try {
      await signIn(readSignIn(text));
      onSignedIn();
    } catch (failure) {
      setError((failure as Error).message);
      setBusy(false);
    }
  }

  // Once only, though development runs every effect twice: a code works once.
  const tried = useRef(false);
  useEffect(() => {
    if (!code || tried.current) return;
    tried.current = true;
    submit(code);
  });

  return (
    <div className="flex h-full items-center justify-center overflow-y-auto bg-surface-0 px-4 py-8">
      <form
        className="flex w-full max-w-sm flex-col gap-4 rounded-lg border border-divider bg-surface-1 p-6"
        onSubmit={(event: FormEvent) => {
          event.preventDefault();
          submit(typed);
        }}
      >
        <div className="flex items-center gap-2">
          <LogoTile />
          <span className="font-display text-[16px] font-bold tracking-tight text-ink-primary [font-stretch:85%]">Agentry</span>
        </div>
        <h1 className="text-[15px] font-semibold text-ink-primary">Sign in</h1>
        <Field
          label="Code, token or link"
          htmlFor="signin"
          hint={
            <>
              On a device that is signed in, open Sign in another device for a code. Or paste the link{" "}
              <span className="font-mono">agentry serve</span> printed.
            </>
          }
        >
          <Input
            id="signin"
            mono
            autoFocus
            autoComplete="off"
            autoCapitalize="none"
            spellCheck={false}
            placeholder="ABCD-EFGH"
            value={typed}
            onChange={(event) => setTyped(event.target.value)}
          />
        </Field>
        {error && <Banner title="Couldn't sign in">{error}</Banner>}
        <Button type="submit" variant="primary" disabled={busy || !typed.trim()}>
          {busy ? "Signing in…" : "Sign in"}
        </Button>
      </form>
    </div>
  );
}
