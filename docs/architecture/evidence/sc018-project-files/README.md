# Project Files 200% zoom evidence

`project-files-200-percent-rtl.png` and `project-files-200-percent-ltr.png` were captured by the real-Chrome `TestBrowserScopedProjectFilesShowsUsageAndPageCounts` gate at actual 200% page zoom (1440px window, 720 CSS-pixel viewport, DPR 2).

The test loads two synthetic file metadata records in temporary SQLite, verifies the localized 2.00 GiB usage summary, searches for one file and clears the search in Arabic RTL and English LTR. Both layouts have a 720px document width with no horizontal overflow. The browser test exposes neither an upload form nor a download link; it does not read or write media bytes. The screenshot handshake captures the page before reporting each locale result.

This evidence covers the Project Files view in a browser emulator. It does not qualify a physical tablet or production Vault.
