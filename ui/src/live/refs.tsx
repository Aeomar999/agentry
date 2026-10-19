import { createContext, ReactNode, useContext, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import type { TaskSummary } from "@/api/types";
import { LinkTo, linkInto, refTo } from "@/lib/links";
import { useResource } from "./use-resource";

// What a task's text can link to besides the web and its files: the board's
// tasks, by id, from the task list the shell reads anyway (so no fetch of
// its own), and the task's commits, read once on the task page.

const Titles = createContext<Map<string, string>>(new Map());

export function TaskTitlesProvider({ tasks, children }: { tasks: TaskSummary[]; children: ReactNode }) {
  const titles = useMemo(() => new Map(tasks.map((task) => [task.id, task.title])), [tasks]);
  return <Titles.Provider value={titles}>{children}</Titles.Provider>;
}

// useTaskCommits is the task's commit hashes; none until they come, or if
// they don't, so a SHA stays plain text.
export function useTaskCommits(taskId: string): string[] {
  const commits = useResource<{ hash: string }[]>(`/api/tasks/${encodeURIComponent(taskId)}/commits`).data;
  return useMemo(() => (commits ?? []).map((commit) => commit.hash), [commits]);
}

// useTaskLinks is Markdown's link and refs hooks for a task's text. A link to
// another task stays in the app, as the router's own links do.
export function useTaskLinks(taskId: string, commits: string[]) {
  const titles = useContext(Titles);
  const navigate = useNavigate();
  return useMemo(() => {
    const refs = { titles, commits };
    const inApp = (to: LinkTo | null): LinkTo | null =>
      to && !to.external && to.href.startsWith("/") ? { ...to, onClick: () => navigate(to.href) } : to;
    return {
      link: (href: string) => inApp(linkInto(taskId, href, refs)),
      refs: (word: string) => inApp(refTo(taskId, word, refs)),
    };
  }, [taskId, titles, commits, navigate]);
}
