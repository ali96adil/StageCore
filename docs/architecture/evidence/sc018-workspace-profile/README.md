# SC-018 Operator dialog zoom evidence

`workspace-profile-200-percent-rtl.png`,
`workspace-profile-200-percent-ltr.png` were captured from the isolated
Chrome-backed `TestBrowserWorkspaceProfileEnterDoesNotDiscardEditorDraft` at
actual 200% browser zoom. Chrome reports a 720 CSS-pixel viewport and DPR 2.
The test confirms there is no document-level horizontal overflow in either
direction and the Save button remains fully inside the dialog after the
dialog scrolls to its action bar. The screenshots show the custom profile
editor in RTL and LTR; the same test sends native Enter in its name field,
verifies that Enter neither closes the editor nor writes localStorage, then
confirms explicit Save stores the new name.

The same real-browser run opens the shared Appearance dialog and captures
`appearance-200-percent-rtl.png` plus `appearance-200-percent-ltr.png`. Its
Close action stays fully within the dialog in both directions. The first
measured views found both action bars clipped by the old negative sticky
offset. The shared `.dialog-actions` rule in `app.css` now keeps both surfaces
inside their dialog scroll frames. The fixture uses only localStorage and
makes no Hub API, Project, rehearsal, runtime, or device request.
