import { lazy, Suspense } from "react";
import type { FileViewProps } from "./MonacoDiff";
import { CodeBlock } from "./CodeBlock";

// A workspace file, in Monaco once it has loaded.
const MonacoDiff = lazy(() => import("./MonacoDiff"));

export function FileView(props: FileViewProps) {
  return (
    <Suspense fallback={<CodeBlock text={props.now} className="h-full rounded-none border-0" />}>
      <MonacoDiff {...props} />
    </Suspense>
  );
}
