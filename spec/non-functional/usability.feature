Feature: Usability
  The page: keys, panels and what the user sees.

  @USE-01 @manual
  Scenario: Ctrl+J opens and closes the dock
    Given the page open and signed in, on any of its pages
    When the user presses Ctrl+J
    Then the dock is shown at the bottom of the content area, never over the top bar
    And the dock is hidden when the user presses Ctrl+J again
    # README.md, Use it, The dock; PLAN.md T-045

  # --- Contrast of the token file: WCAG 2.1, measured, dark and light ---
  # Every ratio below comes from python3 -I .agentry/files/notes/contrast.py ui/src/styles/globals.css, run on
  # 2026-10-25 at commit 45d8d734: the dark tokens are the @theme block (lines 12-96), the light tokens the first
  # html.light block (lines 99-124) over them; color-mix(in srgb, A N%, B) is a linear interpolation of the two
  # colours' sRGB channels; an 8-digit hex is composited over the surface it sits on; the ratio is WCAG 2.1
  # relative luminance, (L1 + 0.05) / (L2 + 0.05), to two decimals. The full table is .agentry/files/notes/usability.md.

  @USE-02 @benchmark
  Scenario: Primary, secondary and the five state inks read at 4.5:1 on every surface in both themes
    Given the token file's four surfaces, surface-0 to surface-3, in dark and in light
    When the contrast of ink-primary, ink-secondary, brand-ink, success-ink, danger-ink, warning-ink and info-ink is computed on each surface
    Then every one of the 28 dark pairs is 4.5:1 or more, the lowest 5.19 for brand-ink on surface-3 and the highest 14.22 for ink-primary on surface-1
    And every one of the 28 light pairs is 4.5:1 or more, the lowest 4.90 for brand-ink and for info-ink on surface-3 and the highest 15.78 for ink-primary on surface-0
    # ui/src/styles/globals.css lines 15-28 and 56-60 (dark), 100-120 (light); measured as the heading above says; commit fc15f1bf raised the inks to AA

  @USE-03 @benchmark
  Scenario: Muted ink reads at 4.5:1 on the base and panel surfaces
    Given ink-muted, the colour of ids, paths and times, on surface-0 and surface-1 in both themes and on surface-2 in dark
    When its contrast is computed
    Then on surface-0 and surface-1 it is 5.34 and 5.62 in dark and 4.83 and 4.63 in light, all 4 pairs 4.5:1 or more
    And on surface-2 in dark it is 4.86, also 4.5:1 or more
    # ui/src/styles/globals.css lines 24-27, 110; measured as the heading above says

  @USE-04 @benchmark
  Scenario: Muted ink is below 4.5:1 on the raised surfaces
    Given ink-muted on surface-3 in dark and on surface-2 and surface-3 in light
    When its contrast is computed
    Then on surface-3 in dark it is 4.24, below 4.5:1 and above 3:1
    And on surface-2 and surface-3 in light it is 4.28 and 3.84, both below 4.5:1 and above 3:1
    And ink-muted is used 215 times in ui/src outside tests, so on those 3 surfaces it reads only as large text would
    # ui/src/styles/globals.css lines 18, 27, 102-103, 110; measured as the heading above says; the uses counted with grep -o text-ink-muted over ui/src/**/*.tsx outside tests

  @USE-05 @benchmark
  Scenario: Faint ink is below 3:1 on every surface in both themes
    Given ink-faint, the colour of placeholders, section titles, hints and chevrons, on each of the four surfaces
    When its contrast is computed
    Then in dark it is 2.84, 2.99, 2.59 and 2.26 on surface-0 to surface-3, every one below 3:1
    And in light it is 2.63, 2.52, 2.32 and 2.08 on surface-0 to surface-3, every one below 3:1
    And ink-faint is used 102 times in ui/src outside tests, 37 at 11 px and 5 at 10.5 px, so no ink-faint text meets 4.5:1 or 3:1 today
    # ui/src/styles/globals.css lines 28, 111; ui/src/components/atoms/Input.tsx line 23 (placeholder), organisms/Sidebar.tsx line 54 (section titles), molecules/Breadcrumbs.tsx line 15 (chevron, aria-hidden); measured as the heading above says

  @USE-06 @benchmark
  Scenario: The link colour reads at 4.5:1 on the base surfaces and is below it on the raised light surfaces
    Given the link token, info itself in dark and info mixed 85 percent toward ink-primary in light
    When its contrast is computed on each surface
    Then in dark it is 7.88, 8.29, 7.17 and 6.26 on surface-0 to surface-3, every one 4.5:1 or more
    And in light it is 5.01 on surface-0 and 4.80 on surface-1, both 4.5:1 or more, and 4.43 on surface-2 and 3.97 on surface-3, both below 4.5:1 and above 3:1
    And the link token has exactly 1 use, a marked link in rendered Markdown; other links are drawn in info
    # ui/src/styles/globals.css lines 49-50, 116; ui/src/components/molecules/Markdown.tsx line 372; measured as the heading above says

  @USE-07 @benchmark
  Scenario: A badge's text reads at 4.5:1 on its soft fill over every surface
    Given a badge in a state's ink on that state's soft fill, an 8-digit hex with alpha 31 of 255 (20 for brand), composited over each surface
    When the contrast of brand-ink, success-ink, danger-ink, warning-ink and info-ink on their fills is computed
    Then every one of the 20 dark pairs is 4.5:1 or more, the lowest 4.70 for brand-ink and for danger-ink on surface-3
    And every one of the 20 light pairs is 4.5:1 or more, the lowest 4.56 for info-ink on surface-3
    # ui/src/styles/globals.css lines 33, 41-47, 56-60, 117-120; ui/src/components/atoms/Badge.tsx lines 8-12; measured as the heading above says; ui/src/components/atoms/Badge.test.tsx asserts the classes, the ink on the soft fill, not the ratio

  @USE-08 @benchmark
  Scenario: The focus line shows at 3:1 on every surface in both themes
    Given the focus colour, brand in dark and brand mixed 80 percent toward ink-primary in light, as a 2 px line on each surface
    When its contrast against the surface is computed, the 3:1 of non-text
    Then in dark it is 5.91, 6.22, 5.38 and 4.70 on surface-0 to surface-3, every one 3:1 or more
    And in light it is 3.95, 3.79, 3.50 and 3.14 on surface-0 to surface-3, every one 3:1 or more
    # ui/src/styles/globals.css lines 62-64, 121, 191-194; measured as the heading above says; commit fc15f1bf

  @USE-09 @benchmark
  Scenario: White on the primary button is below 3:1
    Given the primary button, white text on the brand orange, the same #fc6d26 in both themes
    When its contrast is computed
    Then it is 2.86 in both themes, below the 3:1 of large text and the 4.5:1 of normal text
    And the primary variant is the only button drawn in white on brand, at 12 px or 13 px, so it does not meet AA today
    # ui/src/styles/globals.css line 31; ui/src/components/atoms/Button.tsx lines 15, 22-23; measured as the heading above says

  # --- Phone width ---
  # The breakpoints are Tailwind 4.3.3's defaults, sm 640 px, md 768 px and lg 1024 px; globals.css sets no --breakpoint of its own.
  # No UI test renders at a width, so these are checked by hand at 375 px.

  @USE-10 @manual
  Scenario: At phone width the page never scrolls sideways
    Given the page open at 375 px wide, signed in, in either theme
    When the user opens Tasks as a board, a task, the task's Activity tab and Settings, and reads document.documentElement.scrollWidth and clientWidth in the browser console on each
    Then scrollWidth equals clientWidth on each of the 4 pages, so the page itself does not scroll sideways
    And what is wider than the screen scrolls inside its own box, each with overflow-x-auto: the board's columns, the Plan's columns, the handoff path, a tab list, a code block and a table
    And a toast is at most the screen width minus 32 px wide
    # README.md, Install, What Start Agentry does (a phone can open it); ui/index.html line 5 (viewport width=device-width); ui/src/components/organisms/TaskBoard.tsx line 147, pages/PlanPage.tsx line 128, molecules/HandoffPath.tsx line 41, molecules/Tabs.tsx line 13, molecules/CodeFence.tsx line 29, molecules/ResourceTable.tsx line 57, organisms/Toasts.tsx line 46; no test

  @USE-11 @manual
  Scenario: Under 768 px the sidebar is a drawer and the top bar keeps only its icons
    Given the page open at 375 px wide
    When the user looks at the top bar and presses its first button, "Show or hide the sidebar"
    Then the sidebar, hidden until then, opens as a drawer over the content under the top bar, and a tap on the dark backdrop or on one of its links closes it
    And at 768 px and wider the same button collapses the 240 px sidebar to a 56 px rail instead, with no drawer
    And the top bar shows the logo, the search button as a 32 px icon with no "Search or jump to" text and no key chip, Install app, the theme button and the account menu; the name "Agentry", the divider after it and the breadcrumbs are hidden under 640 px
    # README.md, Use it; ui/src/components/templates/AdminShell.tsx line 181 (matchMedia min-width 768px); organisms/Sidebar.tsx lines 41, 48-49; organisms/TopBar.tsx lines 27-55; ui/src/styles/globals.css lines 84-85; no test

  @USE-12 @manual
  Scenario: Under 1024 px the task list and the task page drop to one column
    Given the page open at 375 px wide, then at 1024 px wide
    When the user opens Tasks as a list, then a task and its Details tab
    Then under 1024 px the list hides the Agent, Turns and Tokens columns, and under 640 px the Active column too; at 1024 px every column is shown
    And under 1024 px the task's Plan rail is a band at most 256 px tall above the content, and at 1024 px a 320 px column beside it
    And under 1024 px the Details tab is 1 column of panels, and at 1024 px 2
    And in the Activity tab the time column is 56 px wide under 640 px, 104 px from 640 px and 120 px from 768 px
    # README.md, Use it, Tasks; README.md, Use it, A task; ui/src/pages/TasksPage.tsx lines 109-120; pages/TaskPage.tsx lines 430-433, 546; organisms/ActivityFeed.tsx line 63; no test

  @USE-13 @test
  Scenario: On a phone a turn's band shows its duration in place of its facts
    Given a task's Activity with a turn that ended after 5 minutes and spent 41k tokens
    When the turn's band is shown
    Then the band holds the turn's id, the machine it ran on and "41k tokens", each in a span shown only at 768 px and wider, and its state "ended" in a badge shown at every width
    And the duration "5m 00s" is in a span shown only under 768 px, so a phone sees the duration and a wider screen sees the facts
    # README.md, Use it, A task; ui/src/components/organisms/ActivityFeed.tsx lines 286-292; ui/src/components/organisms/ActivityFeed.test.tsx

  @USE-14 @manual
  Scenario: A long word in a turn's start reason wraps instead of widening the table
    Given a task whose turn started fresh for a reason that is one word of 80 characters with no space
    When the task page shows the Turns table at 375 px wide
    Then the reason breaks onto more lines under the turn's tokens, in 11 px muted ink, and the table is not widened
    And the page does not scroll sideways
    # README.md, Use it, A task; ui/src/pages/TaskPage.tsx lines 136 (break-words, contain inline-size), 174; commit 0701ba9c; no test

  # --- Keys and focus ---

  @USE-15 @manual
  Scenario: Ctrl+K opens the command palette from anywhere, a field included
    Given the page open with the cursor in a text field
    When the user presses Ctrl+K, or ⌘ K on a Mac
    Then the command palette opens, a dialog named "Command palette", and the key does not reach the field
    And ArrowDown and ArrowUp move the cursor exactly one match at a time, Enter runs the match under it and Esc closes it
    And with an empty query the palette lists every item: Go to Tasks, Agents, Policy, Plan, Integrations and Settings, New task, New epic, Ask Claude to start a task, one Open the terminal command per working task with an agent, the theme switch, the commands of each visible feature, Go to Ideas when Ideas is on, then every task and every agent
    # README.md, Use it, Keys; PLAN.md T-017; ui/src/components/templates/AdminShell.tsx lines 101-115 (keydown with capture) and 127-164; organisms/CommandPalette.tsx lines 38-44 (an empty query matches every item), 60-63, 72-73; lib/shortcuts.ts line 12 (modKey); no test presses the keys, CommandPalette.test.tsx covers the matching of epic-N and task-N

  @USE-16 @manual
  Scenario: Single keys and two-key chords work outside a field, and Esc closes what is open
    Given the page open with focus on the page body, not in a field or the terminal
    When the user presses ?, n, /, g then t, g then a, g then p, 1, 2, j, k or Enter
    Then ? shows the shortcuts overlay listing exactly 14 shortcuts in 4 groups, Anywhere, Go to, Tasks and Lists; n opens New task; / focuses the page's search field; g t, g a and g p go to Tasks, Agents and Policy when the second key comes within 1200 ms; 1 and 2 show Tasks as a list and as a board; j and k move the row cursor and Enter opens the row under it
    And the same keys typed in an input, a textarea, a select or an editable element, or with Ctrl, ⌘ or Alt held, run no shortcut
    And Esc closes the overlay, the palette, a dialog and a context menu; a context menu also closes on a click elsewhere or a scroll, and is kept 8 px inside the window's right edge
    # README.md, Use it, Keys; PLAN.md T-017; ui/src/lib/shortcuts.ts lines 14-29 (the list the overlay shows), 33-37 (isTyping), 49, 59 (1200 ms), 72-80; components/templates/AdminShell.tsx lines 117-125; pages/TasksPage.tsx line 184; organisms/ShortcutOverlay.tsx line 8; molecules/ContextMenu.tsx lines 5, 13, 30; no test presses these keys

  @USE-17 @test
  Scenario: Esc or a press outside closes a dialog, unless it is held
    Given a dialog open, a box with role dialog named by its title, over a dark scrim
    When the user presses the mouse inside it, presses Esc, then presses the mouse on the scrim
    Then the press inside closes nothing, and Esc and the press on the scrim close it, exactly 2 closes
    And while the dialog is held, neither Esc nor a press on the scrim closes it, 0 closes; once let go both close it again
    # ui/src/components/molecules/Dialog.tsx lines 24-28, 33-37; ui/src/components/molecules/Dialog.test.tsx

  @USE-18 @test
  Scenario: A dialog with a request running stays open on Esc until the request ends
    Given the Agents page with the Add a team dialog open and its request still running
    When the user presses Esc or clicks outside it
    Then the dialog stays open while the request runs, and once it ends Esc and a click outside each close it
    And once an add has succeeded and closed it, the dialog opened again is closed by Esc and by a click outside, not held
    # README.md, Use it, Teams; ui/src/pages/AgentsPage.test.tsx

  @USE-19 @test
  Scenario: Closing a team panel with changes asks first and gives focus back
    Given the team panel open with an unsaved change, focus in the team name field
    When the user presses Esc, or presses Close, or presses Cancel
    Then the panel asks first, and focus moves to its "Keep editing" button; the "Create team" button is hidden while it asks
    And Esc again, or Keep editing, puts focus back exactly where it was: the name field, the Close button or the Cancel button
    # README.md, Use it, Teams; ui/src/components/organisms/TeamPanel.test.tsx

  @USE-20 @manual
  Scenario: Focus moved by the keyboard shows one 2 px line, a click shows none
    Given the page open in either theme
    When the user moves focus with Tab to a button, a tab, a table row and a link, then clicks one of them with the mouse
    Then the element reached by Tab shows a 2 px solid line in the focus colour, brand orange in dark and a darker orange in light, and the clicked one shows none
    And the line sits 2 px outside the primary button, and inside where a scroller or a strip would clip it: 3 px inside a tab, 2 px inside a table row and 1 px inside the Activity scroller
    And a text field, textarea or select shows no line of its own; its wrapper's border turns the focus colour instead, so there is only one line
    # PLAN.md T-017; ui/src/styles/globals.css lines 62-64, 121, 186-199; ui/src/components/atoms/Button.tsx line 15; molecules/ResourceTable.tsx line 99; molecules/Tabs.tsx line 24; organisms/ActivityFeed.tsx line 159; atoms/Input.tsx line 9; no test

  @USE-21 @test
  Scenario: Tab in the Activity feed scrolls the focused element wholly into view
    Given the Activity feed, a scroller whose scroll padding keeps a stuck band at its top and, on a phone, padding at its bottom, with links and buttons in its rows and a code block folded to its first lines
    When Tab moves focus to an element under the band, one past the bottom, one already in view, one taller than the room left, and one inside the folded code
    Then the element under the band ends just below it, the one past the bottom just above the padding, and the one already in view moves 0 px
    And the element taller than the room is brought only up to the band, and inside the folded code only the lines above the cut count as what must be in view
    And a click that focuses an element scrolls 0 px
    # README.md, Use it, A task; ui/src/lib/focus-in-view.ts; ui/src/components/organisms/ActivityFeed.tsx lines 148, 159; ui/src/pages/TaskPage.tsx line 464; ui/src/lib/focus-in-view.test.tsx

  @USE-22 @test
  Scenario: The Activity scroller is a Tab stop named by its heading
    Given a task's Activity tab
    When the user tabs into the feed
    Then the scroller is a Tab stop with tabIndex 0, a region named "Activity" through aria-labelledby from the tab's heading, with no aria-label of its own
    And focusing the scroller itself moves it 0 px
    And every agent chip in the bands is hidden from screen readers, aria-hidden true
    # README.md, Use it, A task; ui/src/components/organisms/ActivityFeed.tsx lines 144-159; ui/src/components/organisms/ActivityFeed.test.tsx

  @USE-23 @test
  Scenario: A folded entry keeps only what is visible in the Tab order
    Given a long entry folded to its first lines, with links above and below the cut
    When the user tabs through it, presses Show all, then Show less
    Then folded, Tab reaches only the links above the cut and then the Show all button
    And Show all puts every link back in the Tab order, and Show less takes the ones below the cut out again, each element getting back the tabindex it had
    And the fold never uses the inert attribute
    # README.md, Use it, A task; ui/src/components/molecules/Fold.tsx lines 7, 93, 158; ui/src/components/molecules/Fold.test.tsx (never uses inert, lines 211-216)

  # --- Words, motion and theme ---

  @USE-24 @benchmark
  Scenario: The UI's words carry no em-dash and none of the three banned words
    Given every .ts and .tsx file under ui/src outside tests
    When the user greps them for the em-dash character, and for the three words spec/README.md rule 6 bans
    Then the em-dash is found on exactly 1 line, a code comment in ui/src/lib/signin.ts, and in 0 strings shown to the user
    And the three banned words are found 0 times
    # spec/README.md rule 6; measured 2026-10-25 at commit 45d8d734 with find ui/src -name '*.ts' -o -name '*.tsx' | xargs grep -n for the em-dash character, and grep -rniE for the three words; ui/src/lib/signin.ts line 8

  @USE-25 @manual
  Scenario: Reduced motion turns every animation and transition off
    Given the system set to reduce motion, so prefers-reduced-motion is reduce
    When any page is open
    Then every animation and every transition on every element runs with a duration of 0.01 ms, with !important, so the sidebar's width change and each colour change finish within 1 frame
    # ui/src/styles/globals.css lines 201-208; no test

  @USE-26 @manual
  Scenario: The light theme is one class on the html element and is kept for the next visit
    Given the page in the dark theme, the default
    When the user presses the theme button in the top bar, "Light theme", or runs "Switch to the light theme" from the palette
    Then the html element gets the class light, every token takes its light value, and the browser's theme colour becomes #fafafa from #16181d
    And the choice is kept in localStorage under agentry-theme, so the next visit opens in the same theme, and is posted to POST /api/theme, which accepts only "light" or "dark"
    # PLAN.md T-017, T-052; ui/src/lib/theme.ts lines 7-27; ui/src/components/templates/AdminShell.tsx lines 95, 149-152; organisms/TopBar.tsx line 55; internal/web/api.go lines 510-523; no test
