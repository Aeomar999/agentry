import { createContext, useContext } from "react";

// A page opens the dock on one question with useOpenQuestion, and AdminShell,
// which provides QuestionContext, opens it. It lives apart from AdminShell so
// a page doesn't import the shell that imports it.

export const QuestionContext = createContext<(questionId: string) => void>(() => {});

// useOpenQuestion opens the dock's Questions on one question, tall enough
// for the page the coordinator may have made for it.
export function useOpenQuestion(): (questionId: string) => void {
  return useContext(QuestionContext);
}
