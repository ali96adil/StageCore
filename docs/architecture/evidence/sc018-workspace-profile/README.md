# SC-018 Workspace Profile zoom evidence

`workspace-profile-200-percent-rtl.png` and
`workspace-profile-200-percent-ltr.png` were captured from the isolated
Chrome-backed `TestBrowserWorkspaceProfileEnterDoesNotDiscardEditorDraft` at
actual 200% browser zoom. Chrome reports a 720 CSS-pixel viewport and DPR 2.
The test confirms there is no document-level horizontal overflow in either
direction and the Save button remains fully inside the dialog after the
dialog scrolls to its action bar. The screenshots show the custom profile
editor in RTL and LTR; the same test sends native Enter in its name field,
verifies that Enter neither closes the editor nor writes localStorage, then
confirms explicit Save stores the new name.

The first measured image showed the sticky action bar clipped past the
dialog's bottom edge. `workspace-profile.css` now keeps the F-017 action bar
within the dialog scroll frame. The fixture uses only localStorage and makes
no Hub API, Project, rehearsal, runtime, or device request.
