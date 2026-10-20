import { Banner } from "./Banner";

// Warnings are what the server says may be left for now after an erasure
// or a redaction that went through, in its words.
export function Warnings({ warnings }: { warnings: string[] }) {
  if (warnings.length === 0) return null;
  return (
    <Banner tone="warning" title="Some of it may be left for now">
      <ul className="flex list-disc flex-col gap-1 pl-4">
        {warnings.map((warning) => (
          <li key={warning}>{warning}</li>
        ))}
      </ul>
    </Banner>
  );
}
