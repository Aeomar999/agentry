Feature: Shared files
  Files that are not in git, shared with a task in .agentry/files, that follow it from machine to machine.
  # Sources: internal/domain/sharedfiles, internal/domain/sharedfiles/sharedfilesapi,
  # internal/domain/tasks/files.go, internal/runtime/coordinator/files.go,
  # internal/web/shared_api.go, ui/src/components/organisms/SharedFiles.tsx
  # and their tests.

  # ---- Adding and downloading ------------------------------------------------

  @FILES-01 @user @test
  Scenario: Shared files: add one, download it, take it away
    # internal/domain/sharedfiles/sharedfilesapi/sharedfilesapi_test.go TestYouShareFilesWithATask
    # internal/domain/sharedfiles/sharedfilesapi/sharedfilesapi.go lines 77 to 128
    Given task-1 with no shared files
    When the user shares "data/sales.csv" holding 8 bytes
    Then the list shows the file "data/sales.csv", size 8, by "you"
    And the task's detail lists it first under shared: yes
    When the user downloads it
    Then the body is exactly the 8 bytes shared
    When a file named "../escape" is shared
    Then it is refused with 400
    When the user takes "data/sales.csv" away
    Then the list is empty

  @FILES-02 @user @manual
  Scenario: The Files tab drops files in, up to 2 GB each, and a story also has its epic's files
    # ui/src/components/organisms/SharedFiles.tsx lines 105 to 147
    # 2 GB: internal/domain/sharedfiles/blobs.go lines 22 and 25
    # a story's epic's files, in epic/: internal/domain/sharedfiles/folder.go line 28, internal/runtime/coordinator/files.go line 138
    Given task-5's Files tab is open with no files
    Then "No shared files" is shown with "Drop files here to share them with the task's agents. What they leave in .agentry/files shows here too."
    When the user drops 2 files on the tab
    Then a toast "Shared 2 with task-5" with "The next agent finds them in .agentry/files." is shown once
    And each row shows the file, its size, "you" and a Download link and a "Stop sharing it" button
    When a file over 2 GB is shared
    Then it is refused with "a shared file is at most 2 GB" and the row is not added
    Given a story of epic-7, and epic-7 has 1 shared file
    Then the story's worktree has that file under .agentry/files/epic, to read only

  @FILES-03 @claude-code @test
  Scenario: A shared file's name stays inside the task's folder
    Given a task
    When a file is uploaded under a name that leaves the folder, such as one starting with ..
    Then it is refused with status 400
    # internal/domain/sharedfiles/blobs.go MaxSize: a shared file is at most 2 GB; no test sends one that big
    # internal/domain/sharedfiles/sharedfilesapi/sharedfilesapi_test.go TestYouShareFilesWithATask

  # ---- Following the task ----------------------------------------------------

  @FILES-04 @coordinator @test
  Scenario: Shared files in .agentry/files follow the task
    # internal/runtime/coordinator/files_test.go TestSharedFilesFollowTheWork
    Given an agent's turn added a file under .agentry/files in the worktree
    When the turn ends
    Then the file is read back and kept with the task, recorded with the agent that left it and its size, 9 bytes
    # not under test, checked by hand: the time it was left is recorded too (SharedFile.At)
    And the coordinator's next brief lists it as "out/report.csv (9 bytes, from engineer)"
    And a file the user shares is in the next agent's .agentry/files when its turn starts, and a file a turn removed is shared no more
    # a shared file is Name, Hash, Size, By, At, From: internal/domain/tasks/files.go SharedFile

  @FILES-05 @coordinator @manual
  Scenario: A story's worktree has its epic's files under .agentry/files/epic, to read
    Given a story of an epic, and the epic has exactly 1 shared file, brief.md
    When the story's turn starts
    Then the story's .agentry/files/epic holds exactly that file, and the brief says changes there aren't kept
    And a file written under .agentry/files/epic during the turn is not shared when the turn ends
    # the delivery under epic/: internal/runtime/coordinator/files.go filesFor; the folder, left out of what a turn shares: internal/domain/sharedfiles/folder.go EpicFolder, Manifest
    # the folder left out of the manifest: internal/domain/sharedfiles/sharedfiles_test.go TestFilesGoRoundBetweenWorktrees; the brief's words: internal/runtime/coordinator/brief.go sharedText
    # Steps: drop a file on an epic's Files tab, start one of its stories, and in the story's worktree read
    # .agentry/files/epic/<the file>; write a new file beside it and end the turn; the story's Files tab shows no such
    # file (with no other shared file it says No shared files).

  @FILES-06 @worker @test
  Scenario: A task's shared files follow the work to the next machine
    Given a task's worktree with files in .agentry/files
    When the turn ends and the work moves
    Then the files are sent to the server as blobs by their hash, and the next worktree receives the same files: yes
    # internal/runtime/coordinator/loop.go startOn: the last turn's files are read back where it ran before the work moves
    # internal/domain/sharedfiles/blobs.go MaxSize: a shared file is at most 2 GB
    # internal/domain/sharedfiles/blobs_test.go: the same bytes make the same blob, so a file kept by 2 tasks is stored 1 time
    And a shared file's name may leave .agentry/files: no
    # internal/domain/sharedfiles/sharedfiles_test.go TestFilesGoRoundBetweenWorktrees, TestNamesStayInsideTheFolder;
    # the turn's files kept by their hash on the server: internal/runtime/coordinator/files_test.go TestSharedFilesFollowTheWork

  # ---- Fetching a file -------------------------------------------------------

  @FILES-07 @coordinator @test
  Scenario: A fetch from inside a machine's worktrees comes at once, from anywhere else the user is asked first
    # internal/runtime/coordinator/files_test.go TestSharedFilesFollowTheWork
    Given the coordinator decides {"action": "fetch", "machine": "local", "path": "<a path>", "as": "notes.txt"}
    When the path is outside that machine's worktrees
    Then the user is asked whether to copy it, the first option "Copy it"
    And the file is not shared before the user answers
    And a path inside that machine's worktrees is shared with no question asked
    # not under test, checked by hand: the second option "Don't copy it" (internal/runtime/coordinator/files.go line 170), and the file shared once the user answers "Copy it"
    # CopyFile: internal/runtime/coordinator/files.go fetch

  @FILES-08 @worker @test
  Scenario: A worker fetches a file into the shared files only from inside its worktrees, unless the user allows
    Given a worker whose worktrees live in 1 folder, and a file outside it
    When a coordinator's fetch names the file outside
    Then it is refused as outside the worker's folder, and 0 files are sent
    When the fetch names a folder inside the worktrees
    Then every file in it is sent under the name given, 1 file for 1 file
    When the user has allowed it
    Then the file outside is sent: yes
    # not under test, checked by hand: the user allows it by answering Copy it (internal/runtime/coordinator/files.go line 29); files_test checks the question offers it
    # internal/domain/sharedfiles/sharedfiles_test.go TestFetchAsksOutsideTheBoundary; 0 files before the user allows it:
    # internal/runtime/coordinator/files_test.go TestSharedFilesFollowTheWork
