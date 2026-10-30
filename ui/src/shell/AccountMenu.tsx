import { useRef, useState } from "react";
import { UserRound } from "lucide-react";
import { IconButton } from "@/components/atoms/IconButton";
import { ContextMenu } from "@/components/molecules/ContextMenu";
import { signOut } from "@/api/client";
import { NotifyDeviceDialog } from "@/domains/notifications/NotifyDeviceDialog";
import { SignInDeviceDialog } from "@/shell/signin/SignInDeviceDialog";
import { useToast } from "./Toasts";

// This device's sign-in, in the top bar: sign in another device with a
// one-time code, have this one notified, or sign it out.
export function AccountMenu() {
  const button = useRef<HTMLButtonElement>(null);
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null);
  const [signingIn, setSigningIn] = useState(false);
  const [notifying, setNotifying] = useState(false);
  const notify = useToast();

  function open() {
    const box = button.current!.getBoundingClientRect();
    setMenu({ x: box.right - 240, y: box.bottom + 4 });
  }

  return (
    <>
      <IconButton
        ref={button}
        icon={<UserRound />}
        label="Signing in"
        aria-haspopup="menu"
        onMouseDown={(event) => event.stopPropagation()} // else the open menu closes on it, and the click opens it again
        onClick={() => (menu ? setMenu(null) : open())}
      />
      {menu && (
        <ContextMenu
          x={menu.x}
          y={menu.y}
          onClose={() => setMenu(null)}
          items={[
            { label: "Sign in another device", onClick: () => setSigningIn(true) },
            { label: "Notifications on this device", onClick: () => setNotifying(true) },
            "separator",
            {
              label: "Sign out",
              onClick: () => signOut().catch((failure: Error) => notify("danger", "Couldn't sign out", failure.message)),
            },
          ]}
        />
      )}
      {signingIn && <SignInDeviceDialog onClose={() => setSigningIn(false)} />}
      {notifying && <NotifyDeviceDialog onClose={() => setNotifying(false)} />}
    </>
  );
}
