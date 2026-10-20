import { expect, test, vi } from "vitest";
import { act, render } from "@testing-library/react";
import MonacoView from "./MonacoView";

// Monaco doesn't run in jsdom: a stand-in records the options each editor
// is made with, and can change its options as Monaco does.

const { create, createContextKey, setKey, changeOptions } = vi.hoisted(() => {
  const disposable = { dispose: () => {} };
  const setKey = vi.fn();
  let configured = () => {};
  const editor = {
    getContentHeight: () => 100,
    getValue: () => "",
    setValue: () => {},
    getModel: () => null,
    updateOptions: () => {},
    onDidContentSizeChange: () => disposable,
    onDidChangeModelContent: () => disposable,
    onDidChangeConfiguration: (listener: () => void) => ((configured = listener), disposable),
    createContextKey: vi.fn(() => ({ set: setKey })),
    dispose: () => {},
  };
  return {
    create: vi.fn((_host: HTMLElement, _options: Record<string, unknown>) => editor),
    createContextKey: editor.createContextKey,
    setKey,
    changeOptions: () => configured(),
  };
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

test("after a change of options, Tab moves on again", () => {
  render(<MonacoView text="" language="markdown" onChange={() => {}} tabFocusMode />);
  setKey.mockClear();
  act(() => changeOptions());
  expect(setKey).toHaveBeenCalledWith(true);
});
