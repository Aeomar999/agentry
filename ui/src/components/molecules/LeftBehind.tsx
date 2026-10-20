import { Banner } from "./Banner";

// LeftBehind is what Agentry can't take out after an erasure or a
// redaction (docs/design/log-growth.md section 5, "Snapshots, archives,
// backups and worktrees"), with the time, so you know which backups still
// hold it. Agentry offers no way to remove a task's worktree or branch, so
// the note only names them.
export function LeftBehind({ before, what }: { before: string; what: string }) {
  return (
    <Banner tone="info" title="What Agentry can't take out">
      <ul className="flex list-disc flex-col gap-1 pl-4">
        <li>
          Backups of agentry.db and the data folder made before {before} still hold {what}: Agentry can&apos;t reach them.
        </li>
        <li>The task&apos;s worktree, its local branch and any commits pushed from it are left as they are, for you to remove.</li>
        <li>The engine&apos;s own records, such as its session transcripts, live outside Agentry, which leaves them as they are.</li>
      </ul>
    </Banner>
  );
}
