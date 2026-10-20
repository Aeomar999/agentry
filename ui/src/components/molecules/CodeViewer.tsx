import { Component, lazy, ReactNode, Suspense } from "react";
import { TextArea } from "@/components/atoms/Input";
import { CodeBlock } from "./CodeBlock";

// A file in VS Code's editor: read only, or editable when onChange is
// given. Monaco is large, so it loads only when a page shows one. Until it
// has loaded, or if it fails, the text shows as plain text, so nothing is
// ever hidden behind the editor.

export interface CodeViewerProps {
  text: string;
  language: "ini" | "markdown"; // ini highlights TOML
  maxHeight?: number; // grows with the text up to this, then scrolls
  minHeight?: number;
  onChange?: (text: string) => void; // makes it editable
  readOnly?: boolean; // editable, but not for now, as while a form saves
  placeholder?: string; // shown while the text is empty
  // For an editor that is one field among others, as in the team panel:
  // Tab moves to the next field, and the wheel scrolls past it.
  tabFocusMode?: boolean;
  alwaysConsumeMouseWheel?: boolean;
}

const MonacoView = lazy(() => import("./MonacoView"));

export function CodeViewer(props: CodeViewerProps) {
  const plain = props.onChange ? (
    <TextArea
      value={props.text}
      onChange={(event) => props.onChange!(event.target.value)}
      readOnly={props.readOnly}
      placeholder={props.placeholder}
      rows={16}
      className="font-mono text-[12.5px]"
    />
  ) : (
    <CodeBlock text={props.text} className="border-0 bg-transparent" />
  );
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
