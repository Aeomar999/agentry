import { useState } from "react";
import type { PolicyChange } from "@/api/types";
import { useToast } from "@/components/organisms/Toasts";

// usePolicyChange makes one change to the policy at a time. A toast says
// how it went, with the server's reason when it was refused, and the
// policy is read again either way: a change refused because the file
// moved on then shows the version in force now.
export function usePolicyChange(reload: () => void): {
  busy: boolean;
  run: (done: string, change: () => Promise<PolicyChange>) => Promise<void>;
} {
  const notify = useToast();
  const [busy, setBusy] = useState(false);
  async function run(done: string, change: () => Promise<PolicyChange>) {
    setBusy(true);
    try {
      await change();
      notify("success", done, "The new version is in force now.");
    } catch (failure) {
      notify("danger", "The policy wasn't changed", (failure as Error).message);
    } finally {
      setBusy(false);
      reload();
    }
  }
  return { busy, run };
}
