import { Button } from "@/components/atoms/Button";

interface LoadEarlierProps {
  earlier: boolean; // more events lie before the oldest shown
  loadingEarlier: boolean;
  onLoad: () => void;
}

// The foot of a list of events shown newest first: it reads the page
// before the oldest shown, while there is one.
export function LoadEarlier({ earlier, loadingEarlier, onLoad }: LoadEarlierProps) {
  if (!earlier) return null;
  return (
    <div className="flex justify-center px-6 py-3">
      <Button size="sm" variant="ghost" disabled={loadingEarlier} onClick={onLoad}>
        {loadingEarlier ? "Loading…" : "Load earlier"}
      </Button>
    </div>
  );
}
