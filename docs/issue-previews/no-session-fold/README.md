# Routing: No session ID heading

`fix.patch` is a git-format-patch for local commit
`d845791bef78590da94e50d6c20a187eb4cc95e8`, based on upstream
`674b6ca2881f1289c4ccdec158762978a463320c`. Apply it in a current upstream
checkout with `git am /path/to/fix.patch`. This attachment branch is based
on the older fork main only to host files; do not build or cherry-pick
this attachment commit as the fix. The patch contains the tested source,
regression test, and test README changes.

Both screenshots use synthetic requests: four Codex requests in one session,
three Claude Code requests in another, and six routing-classifier requests
without a session ID. The account is test@example.com. No personal data is used.

- `before.png`: upstream 674b6ca2881f1289c4ccdec158762978a463320c. Clicking
  every group heading leaves six classifier requests visible. The no-session
  heading is a DIV with no arrow or aria-expanded attribute.
- `after.png`: the same frontend with the no-session collapse patch. Clicking
  every heading leaves the three headings and their counts, with zero request
  rows shown. The no-session heading is a BUTTON with aria-expanded=false.

Rendered in Chromium through Playwright 1.62.1, using the actual frontend assets
and mocked API responses. This is not a screenshot of the native Wails window.
