import { useEffect, useState } from "react";
import { Bell, BellOff } from "lucide-react";
import { Button } from "@/components/atoms/Button";
import { Banner } from "@/components/molecules/Banner";
import { Dialog } from "@/components/molecules/Dialog";
import { api } from "@/api/client";
import type { PushStatus } from "@/api/types";
import { browserReason, subscribe, thisDevice, unsubscribe } from "@/lib/push";

// Notifications on this device: subscribes this browser to Agentry's pushes.
// Push works only when the server is reached at an https -public-url and
// the browser has it here; otherwise the reason shows in place of the
// button, the server's first, as fixing it changes the address too.
export function NotifyDeviceDialog({ onClose }: { onClose: () => void }) {
  const [status, setStatus] = useState<PushStatus | null>(null);
  const [subscription, setSubscription] = useState<PushSubscription | null | undefined>(); // undefined: not known yet
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const inBrowser = browserReason();

  useEffect(() => {
    api<PushStatus>("GET", "/api/push")
      .then(async (status) => {
        setStatus(status);
        if (status.available && !inBrowser) setSubscription(await thisDevice(status.publicKey!));
      })
      .catch((failure: Error) => {
        setError(failure.message);
        setSubscription(null); // the button, to try again
      });
  }, [inBrowser]);

  function act(work: () => Promise<void>) {
    setBusy(true);
    setError("");
    work()
      .catch((failure: Error) => setError(failure.message))
      .finally(() => setBusy(false));
  }

  const reason = status && !status.available ? status.reason : inBrowser;
  return (
    <Dialog title="Notifications on this device" onClose={onClose}>
      <div className="flex flex-col gap-4 text-[13px] text-ink-secondary">
        {error && <Banner title="Couldn't change notifications">{error}</Banner>}
        {!status || (!reason && subscription === undefined) ? (
          !error && <p className="text-ink-muted">Reading…</p>
        ) : reason ? (
          <p role="note">{reason}</p>
        ) : subscription ? (
          <>
            <p>This device is notified when something waits for you.</p>
            <div className="flex justify-end">
              <Button
                icon={<BellOff />}
                disabled={busy}
                onClick={() => act(() => unsubscribe(subscription).then(() => setSubscription(null)))}
              >
                Stop notifying this device
              </Button>
            </div>
          </>
        ) : (
          <>
            <p>Get a notification here when an approval or a question waits for you.</p>
            <div className="flex justify-end">
              <Button
                variant="primary"
                icon={<Bell />}
                disabled={busy}
                onClick={() =>
                  act(async () => {
                    setSubscription(await subscribe(status.publicKey!));
                  })
                }
              >
                Notify me on this device
              </Button>
            </div>
          </>
        )}
      </div>
    </Dialog>
  );
}
