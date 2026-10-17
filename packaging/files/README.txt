Agentry
=======

A task board where the workers are AI agents. It runs on this computer and
you use it in your browser.

Start
-----
{{START}}

It starts Agentry in the background and opens it in your browser, signed
in. Close the window whenever you like: Agentry keeps running. Run Start
Agentry again to open the page again.

{{FIRST}}

Stop
----
{{STOP}}

Then, from the page
-------------------
- Add a task: New task (or press n). Describe it in your own words.
- Add machines: the Workers page gives one command to run on another
  computer, so agents can work there too. On a Mac or Linux it runs in a
  terminal; on Windows, in PowerShell.
- Use it from Claude Code: the Integrations page gives one command for the
  computer you use Claude Code on, the same way. On this computer, instead,
  open a terminal in this folder and run ./agentry mcp install (on
  Windows, .\agentry.exe mcp install in PowerShell). Then plain claude, in
  any repo, can hand Agentry work and tell you when it needs you.

What you need
-------------
- git: https://git-scm.com/downloads
- Claude Code, signed in: https://claude.com/claude-code
  (agents run in it; Codex works too, if you have it)

Start Agentry says if either is missing.

Where things are
----------------
- Agentry keeps its log, settings and the tasks' worktrees in .agentry in
  your home folder. Delete that folder to start afresh.
- New tasks work in the git repo Agentry in your home folder, unless you
  give a task another repo, or a new one, on the page.
- The other programs in this folder (agentry-linux-amd64 and so on) are
  what Agentry gives other computers that join as workers, from the
  Workers page. Keep them next to agentry.

Version {{VERSION}}
