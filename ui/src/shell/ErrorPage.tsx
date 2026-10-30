import { useRouteError } from "react-router-dom";
import { Banner } from "@/components/molecules/Banner";

// What a page shows when it breaks: the error, inside the shell, so the
// rest of the UI keeps working.
export function ErrorPage() {
  const error = useRouteError() as Error | undefined;
  return (
    <div className="p-6">
      <Banner title="This page hit an error">
        <span className="font-mono text-[12px] whitespace-pre-wrap">{error?.message ?? String(error)}</span>
        <p className="mt-2">Reload the page to try again. Other pages still work.</p>
      </Banner>
    </div>
  );
}
