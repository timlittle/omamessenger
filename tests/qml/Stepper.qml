import QtQuick

// Stepper is the shared poll-and-retry scaffolding the offscreen QML
// tests use while waiting on the helper, a key event's round trip, or
// Quickshell itself to finish loading before Qt.exit() takes effect.
// make test-qml links this file into every test's root, the same way it
// links Check.js.
//
// A test drives Stepper one of two ways: set steps and let it run them
// in order (each one tried until it returns true, false, or a string
// reason to fail at once — see tests/qml/Flows), or leave steps empty
// and drive its own chain of named check functions with retry(fn)
// instead (see tests/qml/ChatNavigation).
Item {
  id: root

  // name is this test's name, printed on the PASS line.
  property string name: ""
  // steps are tried in order by runStep(), starting once startFn (if
  // set) has run. Leave empty for a test that only uses retry().
  property var steps: []
  // startFn, when set, runs once after startDelayMs, before the first
  // step. Leave it null for a test that starts some other way (see
  // tests/qml/Controllers, which starts once its own Service reports
  // ready).
  property var startFn: null
  // startDelayMs is how long Stepper waits before running startFn;
  // Quickshell ignores Qt.exit() before it has finished loading.
  property int startDelayMs: 50
  // maxAttempts bounds how many times one array step, or one retry()ed
  // check, is retried before timing out.
  property int maxAttempts: 100
  // intervalMs is the delay before a step, or a retry()ed check, runs
  // again.
  property int intervalMs: 100
  // deadlineMs is a backstop that fails the test if nothing above ever
  // finishes; 0 (the default) means no backstop timer at all.
  property int deadlineMs: 0
  // onTimeout runs when the backstop fires.
  property var onTimeout: () => root.fail("timed out")
  // isFailure decides whether an array step's result ends the test at
  // once; a test overrides this when a string result should sometimes
  // be retried rather than failed outright (see tests/qml/NarrowLayout).
  property var isFailure: (result) => typeof result === "string"
  // describeFailure turns a plain reason into the message that is
  // actually logged; a test overrides this to add its own state (see
  // tests/qml/Install).
  property var describeFailure: (reason) => reason

  // step is the array step currently running; attempts is how many
  // times it (or a retry()ed check) has been retried.
  property int step: 0
  property int attempts: 0
  property var _next: null

  // fail stops the test, logging reason (through describeFailure)
  // alongside the array step it happened on.
  function fail(reason: string): void {
    console.error("FAIL step " + root.step + ": " + root.describeFailure(reason));
    Qt.exit(1);
  }

  // pass stops the test successfully.
  function pass(): void {
    console.log("PASS " + root.name);
    Qt.exit(0);
  }

  // retry schedules fn to run again after intervalMs, for a check that
  // depends on a reply from the helper or a key event finishing its
  // round trip.
  function retry(fn: var): void {
    root._next = fn;
    retryTimer.start();
  }

  // runStep runs the current array step and advances, retries or fails.
  function runStep(): void {
    const result = root.steps[root.step]();
    if (root.isFailure(result)) return root.fail(result);

    if (result === true) {
      root.step++;
      root.attempts = 0;
      if (root.step === root.steps.length) return root.pass();
    } else if (++root.attempts > root.maxAttempts) {
      return root.fail(typeof result === "string" ? result
        : "condition not met within " + (root.maxAttempts * root.intervalMs / 1000) + " s");
    }

    stepTimer.start();
  }

  Timer {
    id: stepTimer
    interval: root.intervalMs
    onTriggered: root.runStep()
  }

  Timer {
    id: retryTimer
    interval: root.intervalMs
    onTriggered: root._next()
  }

  Timer {
    running: root.startFn !== null
    interval: root.startDelayMs
    onTriggered: root.startFn()
  }

  // A backstop: it only fires if a step or a retry()ed check hangs
  // without failing, and only when deadlineMs asks for one.
  Timer {
    running: root.deadlineMs > 0
    interval: root.deadlineMs
    onTriggered: root.onTimeout()
  }
}
