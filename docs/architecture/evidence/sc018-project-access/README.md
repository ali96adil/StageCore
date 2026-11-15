# Project Access 200% zoom evidence

The retained RTL screenshot comes from the real-Chrome
`TestBrowserOwnerProjectAccessEditorStoresPreparedLevel` acceptance in
`internal/httpapi/operator_project_access_browser_test.go`. It uses a temporary
Chrome profile with actual 200% page zoom (1440px window, 720 CSS-pixel
viewport, DPR 2) and temporary SQLite. The test verifies the Owner warning,
prepared Project grant fields and Save button remain usable without document or
form overflow. It selects OPERATE only in the unsaved form and asserts that no
API mutation occurs.

The screenshot is `project-access-200-percent-rtl.png`. It captures the Save
control in view; the sticky page header partially covers the section heading
after scrolling, matching the current layout. This evidence does not imply
physical Android/tablet qualification, ACL production activation, or an
applied permission change.
