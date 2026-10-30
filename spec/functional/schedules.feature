Feature: Schedules
  A schedule says when Agentry makes a task: daily, on weekdays or on some days of the week, at a time in a time zone.
  # Sources: internal/extensions/schedules, cmd/agentry/serve.go and their tests.
  # What is built today: the schedule- streams and their 7 event types, the state each stream folds to,
  # the slot a rule makes next, and the reads of every schedule. The rules, the clock that fires a schedule,
  # its routes, MCP tools and page in docs/design/schedules.md are not built yet.

  # ---- A schedule's events and what it keeps ---------------------------------

  @SCHED-01 @user @test
  Scenario: A schedule the user makes keeps every field it was made with, and is on
    # internal/extensions/schedules/schedule_test.go TestACreatedScheduleKeepsEveryField
    # docs/design/schedules.md, 1. What a schedule is; 3. The state slice
    Given a schedule.created on schedule-1 titled "Nightly tidy", described "Tidy the board", for the repo /r and the team tech, every week on mon and thu at 02:00 in Europe/London, starting its tasks at once, with a budget of 5 and a turn limit of 40
    When the stream is folded
    Then the schedule keeps exactly those fields
    And its state is on, and its Since and CreatedAt are the same time, the event's
    And it has made 0 runs, with no last slot and no last task

  @SCHED-02 @user @test
  Scenario: A field the schedule does not know, such as a cron rule, changes nothing
    # internal/extensions/schedules/schedule_test.go TestAFutureFieldIsIgnored
    # docs/design/schedules.md, 1. What a schedule is: a cron rule can come later as an optional field
    Given the same schedule.created twice, once with a "cron" field of "0 2 * * *" and once without
    When each is folded
    Then the two schedules are the same

  @SCHED-03 @user @test
  Scenario: A change carries the whole schedule, so a field it leaves out goes back to its default
    # internal/extensions/schedules/schedule_test.go TestEachTypeFoldsWhatItSays, the case "changed"
    Given a weekly schedule with a description, start, a budget of 5 and a turn limit of 40, that has made 1 run, task-1
    When a schedule.changed makes it "Weekly tidy", daily at 03:30 in Asia/Kolkata, for the repo /s and the team ops, with no description, start, budget or turn limit
    Then the title, repo, team and rule are exactly the changed ones
    And the description is empty, start is not set, and the budget and turn limit are not set, so the server's defaults hold
    And Since moves to the change's time, and the 1 run and its task-1 are kept

  @SCHED-04 @user @test
  Scenario: Pausing, resuming and removing change only the state, and resuming moves Since
    # internal/extensions/schedules/schedule_test.go TestEachTypeFoldsWhatItSays, the cases "paused", "resumed" and "removed"
    # docs/design/schedules.md, 2. Streams and event types: resumed is on again, from now
    Given a schedule that is on and has made 1 run
    When a schedule.paused with the note "away" is folded
    Then its state is paused and nothing else changes
    When a schedule.resumed is folded instead
    Then its state is on and Since is the resume's time, and nothing else changes
    When a schedule.removed is folded instead
    Then its state is removed and nothing else changes

  @SCHED-05 @user @test
  Scenario: A run that made a task moves the last slot, counts the run and names the task
    # internal/extensions/schedules/schedule_test.go TestEachTypeFoldsWhatItSays, the cases "fired" and "fired with no task"
    Given a schedule that has made 1 run, task-1
    When a schedule.fired for task-4 at the slot 2026-10-22T02:00:00+01:00, with 2 missed, is folded
    Then the last slot is that slot, the last task is task-4 and the runs are 2
    And the last run says task-4 at that slot with 2 missed, and the tasks it named are exactly task-1 and task-4
    When a schedule.fired with that slot and no task is folded instead
    Then the runs are 2, the last task is empty and no task is added to the ones it named

  @SCHED-06 @user @test
  Scenario: A run the user asked for with Run now counts, and leaves the last slot where it was
    # internal/extensions/schedules/schedule_test.go TestEachTypeFoldsWhatItSays, the cases "fired by Run now" and "fired with a slot that can't be read"
    # docs/design/schedules.md, 2. Streams and event types: slot is "" for a run you asked for
    Given a schedule that has made 1 run, task-1, at its last slot
    When a schedule.fired by the user for task-4, with an empty slot and manual, is folded
    Then the last slot is the same as before, the last task is task-4 and the runs are 2
    And the last run is manual, with no slot
    When a schedule.fired with the slot "Thursday", which is not a time, is folded instead
    Then the last slot is the same as before and the runs are 2

  @SCHED-07 @user @test
  Scenario: A run skipped because a task is open or was refused moves the last slot and counts no run
    # internal/extensions/schedules/schedule_test.go TestEachTypeFoldsWhatItSays, the cases "skipped while a task is open" and "skipped as refused"
    # TestNamedGrowsOnlyByAFiringAndAnUnrecordedSkip in the same file
    Given a schedule that has made 1 run, task-1
    When a schedule.skipped at the slot 2026-10-22T02:00:00+01:00 with the reason open and task-2 is folded
    Then the last slot is that slot, the last run says open and task-2, and the runs are still 1
    And the tasks it named are not changed: task-1 only
    When a schedule.skipped with the reason refused, task-4 and the why "no such team" is folded instead
    Then the last run says refused, task-4 and "no such team", and the runs are still 1

  @SCHED-08 @user @test
  Scenario: A run a crash cut short, recorded later as unrecorded, names its task and leaves the last slot
    # internal/extensions/schedules/schedule_test.go TestEachTypeFoldsWhatItSays, the cases "skipped as unrecorded" and "skipped as unrecorded, with a slot"
    # TestNamedGrowsOnlyByAFiringAndAnUnrecordedSkip in the same file
    # docs/design/schedules.md, 3. The state slice: Named is how the job tells a recorded run from one a crash cut short
    Given a schedule that has made 1 run, task-1
    When a schedule.skipped with the reason unrecorded, task-3 and the why "made before a crash" is folded
    Then the tasks it named are exactly task-1 and task-3, and the runs are still 1
    And the last slot is not moved, even when the skip carries a slot
    And of every event a schedule takes, only a schedule.fired and an unrecorded schedule.skipped add a task to the ones it named

  @SCHED-09 @user @test
  Scenario: Events before a schedule.created make no schedule, and an event type it does not know changes nothing
    # internal/extensions/schedules/schedule_test.go TestNextBeforeTheScheduleIsCreated, and TestEachTypeFoldsWhatItSays, the case "an unknown type"
    Given a schedule-1 stream whose first event is a schedule.paused
    When it is folded up to that event
    Then there is no schedule
    Given a schedule that is on
    When a schedule.renamed, a type no version has, is folded
    Then the schedule is the same as before

  @SCHED-10 @user @test
  Scenario: Folding one event at a time agrees with folding the whole stream, and never changes the schedule it started from
    # internal/extensions/schedules/schedule_test.go TestNextAgreesWithFromEvents and TestEachTypeFoldsWhatItSays
    Given a stream of 12 events: in log order, a pause before the schedule exists, the schedule.created, a firing, a skip as open, a skip as unrecorded, a pause, a change, a resume, an unknown type, a Run now, a skip as refused and a removal
    When it is folded one event at a time
    Then after each event the schedule is the same as folding the stream up to it in one go
    And the schedule each step started from is not changed, the tasks it named included
    And at the end the schedule is removed, with exactly 2 runs and 3 tasks named

  # ---- When a schedule runs ---------------------------------------------------

  @SCHED-11 @user @test
  Scenario: A daily, a weekdays and a weekly rule each make their next slot strictly after the time given
    # internal/extensions/schedules/rule_test.go TestNextRunsDailyOnWeekdaysAndWeekly
    # docs/design/schedules.md, 1. What a schedule is
    Given a rule daily at 02:00 in Europe/London, on British Summer Time on Monday 19 Oct 2026
    When the next slot is asked for exactly at Monday's 02:00
    Then the slot is Tuesday 20 Oct at 02:00, not Monday's again
    Given a rule on weekdays at 02:00 in Europe/London
    When the next slot is asked for at Friday 23 Oct's 02:00, or on the Saturday or the Sunday
    Then the slot is Monday 26 Oct at 02:00, on Greenwich Mean Time
    Given a rule every week on thu and mon at 09:00 in Europe/London
    When the next slot is asked for exactly at Monday 19 Oct's 09:00
    Then the slot is Thursday 22 Oct at 09:00
    And every slot is given in the rule's own zone

  @SCHED-12 @user @test
  Scenario: Across London's clock changes a time the clocks skip runs as the gap ends, and a time they repeat runs once
    # internal/extensions/schedules/rule_test.go TestNextSettlesLondonsClockChanges
    Given a rule daily at 01:30 in Europe/London
    When the clocks go forward from 01:00 to 02:00 on 29 Mar 2026
    Then that day's slot is 02:00 British Summer Time, 01:00 UTC, as the gap ends, and the next day's is 01:30 as usual
    When the clocks go back from 02:00 to 01:00 on 25 Oct 2026
    Then that day's slot is the first 01:30, at +01:00, and the second 01:30 makes no slot
    And the next slot after the first 01:30 is 26 Oct at 01:30

  @SCHED-13 @user @test
  Scenario: A rule works in a zone half an hour off UTC, through its clock change
    # internal/extensions/schedules/rule_test.go TestNextWorksInHalfHourZones
    Given a rule daily at 09:00 in Asia/Kolkata, UTC+05:30
    When the next slot is asked for at 00:00 UTC on 19 Oct 2026
    Then the slot is 03:30 UTC that day
    Given a rule daily at 02:30 in Australia/Adelaide, whose clocks skip from 02:00 to 03:00 on 4 Oct 2026
    When the next slot is asked for on 3 Oct at 12:00 UTC
    Then the slot is 4 Oct at 03:00 local time, 16:30 UTC on 3 Oct, as the gap ends

  @SCHED-14 @user @test
  Scenario: A daily rule makes exactly one slot on each day of a year, through both clock changes
    # internal/extensions/schedules/rule_test.go TestNextMakesOneSlotADay
    Given a daily rule in Europe/London at each of 00:30, 01:00, 01:30, 02:00 and 02:30
    When its slots are followed one after another through 2026
    Then each day of 2026 has exactly 1 slot, on that day in London, and no day is missed

  @SCHED-15 @user @test
  Scenario: A slot in a gap that ends at midnight runs at 00:00 on the next day
    # internal/extensions/schedules/rule_test.go TestNextMovesASlotInAGapEndingAtMidnightToTheNextDay
    Given a rule every week on sat at 23:30 in America/Nuuk, whose clocks skip from Saturday 23:00 to Sunday 00:00 on the night of 28 to 29 Mar 2026
    When the next slot is asked for on Saturday 28 Mar
    Then the slot is Sunday at 00:00, -01:00
    And the next one is the Saturday after, 4 Apr, at 23:30
    And a daily rule at 23:30 there runs Saturday 28 Mar's slot at Sunday 00:00, and Sunday's at 23:30 as usual

  @SCHED-16 @user @test
  Scenario: A rule that cannot be read makes no slot, so its schedule never fires
    # internal/extensions/schedules/rule_test.go TestNextOfAnUnusableRuleIsZero
    # nothing refuses such a rule when it is written: the schedules extension has no rules yet, see the Feature's notes
    Given a rule every week on mon at 02:00 in Europe/London, which makes a slot
    When it is changed to one of: every "month" or empty; a week with no days or with the day "monday"; at "0200", "2:00", "24:00", "02:60", "02:00am" or empty; the zone "Mars/Olympus_Mons" or empty
    Then the rule makes no slot, in each of the 12 cases

  # ---- Reading the schedules ---------------------------------------------------

  @SCHED-17 @user @test
  Scenario: Every schedule is read oldest first, removed ones too, and a broken one is left out
    # internal/extensions/schedules/list_test.go TestGetAndAllReadTheSchedules
    Given schedule-1 that is on, schedule-2 whose stream is broken, schedule-3 that is removed and schedule-4 with an empty stream
    When one schedule is read
    Then schedule-1 is read, schedule-2 gives its error, and schedule-4 and schedule-9 give no schedule and no error
    When every schedule is read
    Then exactly 2 come back, schedule-1 then schedule-3

  @SCHED-18 @user @test
  Scenario: Schedules read from the log are the same after a restart
    # internal/extensions/schedules/equal_test.go TestALogWithEveryScheduleEventEqualsAReplay
    # the extension is installed by agentry serve: cmd/agentry/serve.go, schedules.Extension
    Given 3 schedules written through the store beside the tasks they made: one fired by the clock, then skipped as open, unrecorded and refused; one paused, changed, resumed and run with Run now; one removed
    When every schedule is read
    Then exactly 3 come back: the first with 1 run and 2 tasks named, the second on with a manual last run, the third removed
    And the state kept event by event, and its snapshots, are the same as a replay of the log
    When Agentry is restarted on the same log
    Then every schedule reads the same as before

  @SCHED-19 @user @test
  Scenario: A build without schedules skips their streams and counts them, and reads the tasks the same
    # internal/core/current/unknown_test.go TestSchedulesStreamsAreSkippedAndCountedWithoutSchedules
    Given a log with 2 schedules and 5 schedule events, a task one of them made, and 2 idea streams, written by a build with schedules
    When a build with neither ideas nor schedules loads it, or applies it event by event
    Then the unknown tally counts exactly 2 schedule- streams with 5 events, beside the ideas'
    And the tasks, the rosters and the settings are the same as the build with schedules makes of it
    And no schedule is read, and schedule-1 is not read as a task

  @SCHED-20 @user @manual
  Scenario: Only the user makes, changes, pauses, resumes and removes a schedule; only Agentry skips a run
    # internal/extensions/schedules/events.go, types; the catalogue's CheckShape and CheckWriter: internal/core/events/catalogue.go
    # no test covers the schedule types here: TestCheckShapeAndWriter in internal/core/events/catalogue_test.go checks only the core's and the ideas' types
    # the steps: build a catalogue with events.NewCatalogue(extensions.Registration(schedules.Extension())) and pass each proposal below to CheckShape, then CheckWriter
    Given a catalogue with the schedules extension
    When an agent proposes a schedule.created, schedule.changed, schedule.paused, schedule.resumed or schedule.removed
    Then each is refused by the writer check
    When the user proposes a schedule.skipped
    Then it is refused by the writer check
    When Agentry, as a System actor, or the user proposes a schedule.fired with a task and a slot
    Then it is allowed
    When the user proposes a schedule.created without a zone, or any of title, repo, team, every and at
    Then it is refused by the shape check
