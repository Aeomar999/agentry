Feature: Notifications
  Pushes to the user's devices when something newly waits for them.
  # Sources: cmd/agentry/push.go, internal/domain/notifications/webpush, internal/domain/notifications/notificationsapi,
  # ui/src/components/organisms/NotifyDeviceDialog.tsx, ui/src/lib/push.ts,
  # ui/public/sw.js and their tests.

  # ---- Turning notifications on ----------------------------------------------

  @NOTIFY-01 @user @test
  Scenario: Notifications on this device need an https public address and a browser that takes pushes
    # internal/domain/notifications/notificationsapi/notificationsapi_test.go TestPushNeedsAnHTTPSPublicURL
    # ui/src/components/organisms/NotifyDeviceDialog.test.tsx "with push off at the server, its reason shows and there is no button", "with push on, the button shows" and "with push on but a browser without it, the browser's reason shows and there is no button"
    # the reasons: internal/domain/notifications/webpush/webpush.go lines 234 to 236 and ui/src/lib/push.ts lines 14 and 17
    Given a server started without -public-url
    When the user opens "Notifications on this device"
    Then push is not available: the note reads "Push needs an https address. Start Agentry with -public-url https://… so phones can reach it." and no button is shown
    Given -public-url over plain http
    Then the reason names that address, and subscribing is refused with 409 and the reason
    Given -public-url over https
    Then push is available with the server's public key, and the button "Notify me on this device" is shown
    Given a browser with no push, or notifications blocked for the site
    Then the browser's reason is shown instead and no button

  @NOTIFY-02 @user @manual
  Scenario: Turning notifications on and off for this device
    # the server half: internal/domain/notifications/notificationsapi/notificationsapi_test.go TestSubscribingTwiceAndUnsubscribing and TestAMalformedPushSubscriptionIsABadRequest
    # the dialog's half when it opens: ui/src/components/organisms/NotifyDeviceDialog.test.tsx "a subscription made with the server's key is sent again and shows as notified" and "a subscription made with another key is dropped in the browser and the button shows"; no test clicks either button
    # the dialog "Notifications on this device", from the Signing in menu: ui/src/shell/AccountMenu.tsx line 41; its buttons: ui/src/components/organisms/NotifyDeviceDialog.tsx lines 52 to 77
    # "Notifications weren't allowed." when the browser refuses permission, in the banner "Couldn't change notifications": ui/src/lib/push.ts line 43; ui/src/components/organisms/NotifyDeviceDialog.tsx line 45
    Given push available and this browser not subscribed
    When the user opens "Notifications on this device" from the Signing in menu, clicks "Notify me on this device" and allows notifications
    Then the browser's subscription is sent and the dialog reads "This device is notified when something waits for you." with "Stop notifying this device" shown
    When the same endpoint is sent twice
    Then exactly 1 subscription is kept
    When the user clicks "Stop notifying this device"
    Then the subscription is removed, 0 subscribed, and removing it again is 404
    When a subscription with no endpoint, an http endpoint or no keys is sent
    Then each is refused with 400 and none is kept
    Given a subscription this browser made with another server's key
    Then it is dropped in the browser, nothing is sent, and the button is shown again

  # ---- What arrives ----------------------------------------------------------

  @NOTIFY-03 @user @test
  Scenario: What arrives: 1 push per device for each thing that newly waits for you, and tapping it opens the task
    # internal/domain/notifications/webpush/sink_test.go TestWhatWaitsAtStartIsNotPushed, TestANewEntryIsPushedOnceToEachDevice and TestTwoNewEntriesAreTwoPushes
    # ui/src/lib/sw.test.ts "shows its title and body, tagged, with the page to open", "opens its page when no window of the app is open", "shows its page in the app's open window" and "opens only Agentry, whatever address the push names"
    # a push carries at most 500 characters of the line: internal/domain/notifications/webpush/sink.go line 15
    Given 2 devices subscribed and an approval already waiting when the server starts
    When the server starts
    Then nothing is pushed for what already waited
    When a new approval waits on task-4
    Then each device gets exactly 1 push: title "Agentry", body "task-4: Waiting for you to approve the push to dev", opening /tasks/task-4
    And checking again pushes nothing more
    When an approval and 3 open ideas newly wait at once
    Then exactly 2 pushes go out, the ideas one opening /
    When a push arrives in the browser
    Then a notification is shown with its title, body and tag; one that cannot be read shows "Agentry" with an empty body
    When the user taps it
    Then its page opens in the app's open window, or in a new one; a push naming another site opens only Agentry's start page
