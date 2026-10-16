import { Component, lazy, ReactNode, Suspense } from "react";
import { CodeBlock } from "./CodeBlock";

// A file shown in VS Code's editor, read only. Monaco is large, so it loads
// only when a page shows one. Until it has loaded, or if it fails, the text
// shows as plain text: the file is never hidden behind the editor.

export interface CodeViewerProps {
  text: string;
  language: "ini" | "markdown"; // ini highlights TOML
  maxHeight?: number; // grows with the text up to this, then scrolls
}

const MonacoView = lazy(() => import("./MonacoView"));

export function CodeViewer(props: CodeViewerProps) {
  const plain = <CodeBlock text={props.text} className="border-0 bg-transparent" />;
  return (
    <PlainOnError fallback={plain}>
      <Suspense fallback={plain}>
        <MonacoView {...props} />
      </Suspense>
    </PlainOnError>
  );
}

class PlainOnError extends Component<{ fallback: ReactNode; children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  render() {
    return this.state.failed ? this.props.fallback : this.props.children;
  }
}
