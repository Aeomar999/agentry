import { useCallback, useEffect, useRef, useState } from "react";
import { RefreshCw } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { QRCode } from "@/components/atoms/QRCode";
import { Select } from "@/components/atoms/Select";
import { Banner } from "@/components/molecules/Banner";
import { Dialog } from "@/components/molecules/Dialog";
import { newSigninCode } from "@/api/client";
import type { SigninCode } from "@/api/types";
import { useNow } from "@/live/use-now";

// Signs in another device, such as a phone or the installed app, without
// copying the token onto it: a one-time code to type there, and a QR code
// of the link that uses it. The server has an address per network, so pick
// the one the device reaches (hidden when there is only one).
export function SignInDeviceDialog({ onClose }: { onClose: () => void }) {
  const [issued, setIssued] = useState<SigninCode | null>(null);
  const [expires, setExpires] = useState(0); // on this browser's clock
  const [url, setURL] = useState("");
  const [error, setError] = useState("");
  const now = useNow(issued !== null);

  // Only the latest request's reply counts: New code clicked twice can be
  // answered out of order.
  const latest = useRef(0);
  const newCode = useCallback(() => {
    const request = ++latest.current;
    setError("");
    newSigninCode()
      .then((code) => {
        if (request !== latest.current) return;
        setIssued(code);
        setExpires(Date.now() + code.expiresIn * 1000);
        setURL((picked) => (code.addresses.some((a) => a.url === picked) ? picked : code.address));
      })
      .catch((failure: Error) => {
        if (request === latest.current) setError(failure.message);
      });
  }, []);
  useEffect(newCode, [newCode]);

  const secondsLeft = issued ? Math.max(0, Math.round((expires - now) / 1000)) : 0;
  const expired = issued !== null && secondsLeft === 0;
  return (
    <Dialog title="Sign in another device" onClose={onClose}>
      <div className="flex flex-col gap-4 text-[13px] text-ink-secondary">
        {error && <Banner title="Couldn't make a code">{error}</Banner>}
        <p>On the other device, open Agentry and enter this code, or scan the QR code with its camera.</p>
        <div className="flex flex-wrap items-center gap-4">
          {issued ? (
            <QRCode
              text={`${url.replace(/\/$/, "")}/?signin=${issued.code}`}
              label="QR code that signs in another device"
              className={expired ? "size-40 opacity-25" : "size-40"}
            />
          ) : (
            <div className="size-40 rounded-md bg-surface-2" />
          )}
          <div className="flex flex-col gap-1">
            <span className="font-mono text-[28px] font-semibold tracking-wider text-ink-primary" aria-live="polite">
              {issued?.code ?? "····-····"}
            </span>
            <span className={expired ? "text-danger" : "text-ink-muted"}>
              {issued &&
                (expired
                  ? "Expired: make a new code"
                  : `Expires in ${Math.floor(secondsLeft / 60)}:${String(secondsLeft % 60).padStart(2, "0")}`)}
            </span>
          </div>
        </div>
        {issued && issued.addresses.length > 1 && (
          <Select
            aria-label="The address the other device reaches"
            className="text-[12px]"
            value={url}
            onChange={(e) => setURL(e.target.value)}
          >
            {issued.addresses.map((a) => (
              <option key={a.url} value={a.url}>
                {a.url.replace(/^https?:\/\//, "")} · {a.label}
              </option>
            ))}
          </Select>
        )}
        <p className="text-[12px] text-ink-muted">The code works once, for five minutes. A new code doesn't stop this one working.</p>
        <div className="flex justify-end gap-2">
          <Button icon={<RefreshCw />} onClick={newCode}>
            New code
          </Button>
          <Button variant="primary" onClick={onClose}>
            Done
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
