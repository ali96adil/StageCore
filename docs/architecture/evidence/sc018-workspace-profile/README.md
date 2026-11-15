# SC-018 Workspace Profile zoom evidence

`workspace-profile-200-percent-rtl.png` was captured from the isolated
Chrome-backed `TestBrowserWorkspaceProfileEnterDoesNotDiscardEditorDraft` at
actual 200% browser zoom. Chrome reports a 720 CSS-pixel viewport and DPR 2.
The test confirms there is no document-level horizontal overflow and the Save
button remains fully inside the dialog after the dialog scrolls to its action
bar. The screenshot is RTL and shows the unsaved custom profile editor; the
same test sends native Enter in its name field, verifies that Enter neither
closes the editor nor writes localStorage, then confirms explicit Save stores
the new name.

The first measured image showed the sticky action bar clipped past the
dialog's bottom edge. `workspace-profile.css` now keeps the F-017 action bar
within the dialog scroll frame. The fixture uses only localStorage and makes
no Hub API, Project, rehearsal, runtime, or device request.
