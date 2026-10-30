import { createContext, ReactNode, useCallback, useContext, useState } from "react";
import { X } from "lucide-react";
import { cn } from "@/lib/cn";

// Toasts: a stack at the bottom right, the title coloured by kind, gone
// after a few seconds. Every action that changes something fires one.

type Tone = "success" | "danger" | "info";

interface Toast {
  id: number;
  tone: Tone;
  title: string;
  detail?: string;
}

type Notify = (tone: Tone, title: string, detail?: string) => void;

const ToastContext = createContext<Notify>(() => {});

const TONES: Record<Tone, string> = {
  success: "text-success",
  danger: "text-danger",
  info: "text-info",
};

let nextId = 1;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const dismiss = useCallback((id: number) => setToasts((all) => all.filter((t) => t.id !== id)), []);
  const notify = useCallback<Notify>(
    (tone, title, detail) => {
      const id = nextId++;
      setToasts((all) => [...all.slice(-3), { id, tone, title, detail }]);
      window.setTimeout(() => dismiss(id), tone === "danger" ? 8000 : 4000);
    },
    [dismiss],
  );

  return (
    <ToastContext.Provider value={notify}>
      {children}
      <div
        aria-live="polite"
        className="pointer-events-none fixed right-4 bottom-10 z-50 flex w-80 max-w-[calc(100vw-2rem)] flex-col gap-2"
      >
        {toasts.map((toast) => (
          <div
            key={toast.id}
            className={cn(
              "pointer-events-auto flex gap-2 rounded-md border border-divider bg-surface-2 py-2.5 pr-2 pl-4",
              TONES[toast.tone],
            )}
          >
            <div className="min-w-0 flex-1">
              <p className={cn("text-[13px] font-medium", TONES[toast.tone])}>{toast.title}</p>
              {toast.detail && <p className="mt-0.5 text-[12px] break-words text-ink-muted">{toast.detail}</p>}
            </div>
            <button
              type="button"
              aria-label="Dismiss"
              onClick={() => dismiss(toast.id)}
              className="size-6 shrink-0 cursor-pointer rounded-sm text-ink-muted hover:bg-surface-3 hover:text-ink-primary"
            >
              <X className="mx-auto size-3.5" />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): Notify {
  return useContext(ToastContext);
}
