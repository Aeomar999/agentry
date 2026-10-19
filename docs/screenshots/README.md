# Screenshots

The README's screenshots of the running page. To retake them:

1. **Screen.** Take full-screen shots of a browser window maximised on a 2880x1800 display, with the page at its default zoom. The originals are 2877x1800, which shows the page about 2000 CSS pixels wide. Keep the page's dark theme.
2. **Pages.** Use a board with real tasks in every column, and take:
   - `tasks-board.png`: Tasks, Board view, with Backlog shown.
   - `task-page.png`: a task with a few rounds behind it, on its Handoffs tab, with the plan open on the left.
   - `agents-org-chart.png`: Agents, Org chart view.
   - `agents-teams-claude.png`: Agents, Teams view, with the Claude panel open at the bottom (Ctrl J) on its Claude tab.
3. **Prepare.** Put the four originals in one folder under those names and run, from the repo root:

   ```sh
   python3 docs/screenshots/prepare.py <folder> docs/screenshots
   ```

   It crops off the desktop bar and the browser's title bar, covers personal details with flat boxes, scales each to 1600 px wide and saves an optimised PNG. It needs Pillow.
4. **Check.** Open each output at full size and read it all. The cover boxes in `prepare.py` are pixel positions in the originals, so new shots need new boxes. Hide:
   - machine names, on the agent cards and anywhere in handoff text;
   - your home folder and any path under it: the status bar, the repo path under a task's title, and paths in briefs and reports;
   - sign-in tokens or `?token=` links, email addresses and personal names;
   - Claude Code's "Transcript saving is off" warning, if it shows.

Each file should stay under about 500 KB.
