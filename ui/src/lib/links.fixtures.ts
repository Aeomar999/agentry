// Links for the tests of the link check, and of what draws links.

// The unsafe links of flows.md's Flow 3 table, and a few more ways to write them.
export const UNSAFE = [
  "javascript:alert(1)",
  "JaVaScRiPt:alert(1)",
  "\u0001javascript:alert(1)",
  " javascript:alert(1)",
  "\u0000 \tjavascript:alert(1)",
  "java\tscript:alert(1)",
  "java\nscript:alert(1)",
  "java\u007Fscript:alert(1)",
  "  JAVASCRIPT:alert(1)  ",
  "data:text/html,<b>x</b>",
  "DATA:text/html,<b>x</b>",
  "vbscript:msgbox(1)",
  " VBScript:msgbox(1)",
  "file:///etc/passwd",
  "blob:https://x/1",
  "foo:bar",
  // A scheme behind a prefix that a repo path drops: still the scheme.
  "/javascript:alert(1)",
  "/JaVaScRiPt:alert(1)",
  "./data:text/html,x",
  "\\javascript:alert(1)",
  "/\tjavascript:alert(1)",
  "a/../javascript:alert(1)",
  "%2e/javascript:x",
  "%2E/VBScript:msgbox(1)",
];

// Hosts written without a scheme, as browsers read them: another site.
export const OTHER_SITES = ["//evil.example", "\\\\evil.example", "/\\evil.example", "\\/evil.example"];
