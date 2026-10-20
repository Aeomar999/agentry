import { expect, test, vi } from "vitest";
import { render } from "@testing-library/react";
import MonacoView from "./MonacoView";

// Monaco doesn't run in jsdom: a stand-in records the options each editor is made with.

const { create, createContextKey } = vi.hoisted(() => {
  const disposable = { dispose: () => {} };
  const editor = {
    getContentHeight: () => 100,
    getValue: () => "",
    setValue: () => {},
    getModel: () => null,
    updateOptions: () => {},
    onDidContentSizeChange: () => disposable,
    onDidChangeModelContent: () => disposable,
    onDidChangeConfiguration: () => disposable,
    createContextKey: vi.fn(() => ({ set: () => {} })),
    dispose: () => {},
  };
  return { create: vi.fn((_host: HTMLElement, _options: Record<string, unknown>) => editor), createContextKey: editor.createContextKey };
});

vi.mock("@/lib/monaco", () => ({ monaco: { editor: { create, setTheme: () => {} } }, defineTheme: () => "agentry" }));

const madeWith = () => create.mock.calls[create.mock.calls.length - 1][1] as { tabFocusMode: boolean; scrollbar: Record<string, unknown> };

test("an editor keeps Tab and the wheel unless asked otherwise", () => {
  render(<MonacoView text="" language="markdown" onChange={() => {}} />);
  expect(madeWith().tabFocusMode).toBe(false);
  expect(madeWith().scrollbar.alwaysConsumeMouseWheel).toBe(true);
  expect(createContextKey).not.toHaveBeenCalled();
});

test("an editor among other fields lets Tab move on and the wheel scroll past it", () => {
  render(<MonacoView text="" language="markdown" onChange={() => {}} tabFocusMode alwaysConsumeMouseWheel={false} />);
  expect(madeWith().tabFocusMode).toBe(true);
  expect(madeWith().scrollbar.alwaysConsumeMouseWheel).toBe(false);
  expect(createContextKey).toHaveBeenCalledWith("editorTabMovesFocus", true);
});
