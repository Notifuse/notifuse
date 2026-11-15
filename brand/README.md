# Notifuse logos

- `notifuse-horizontal-*.svg`: the wordmark, the word "Notifuse" in its frame.
- `notifuse-square-*.svg`: the icon, the N in its frame. "Square" names the icon's shape: it is not the
  `logo-square.png` of the docs and website repositories, which is the wordmark on a white square canvas.
- `notifuse-square-color-padded.svg`: the icon with a margin, for round crops (avatars, social networks).
  It exists in colour only.

The wordmark and the icon each come in `color`, `black` and `white`, with the same geometry. The console draws the same paths in
`console/src/components/brand/NotifuseLogo.tsx`; `NotifuseLogo.test.tsx` checks that they stay in step.
