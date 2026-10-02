## PR-112b-enable: chore(replay): enable Pro Replay UI by default

🎯 Objective

PR-112a (`?asOf=`) and PR-112b (scrubber) are on `dev`. Flip
`kReplayUiEnabled` default to true so Pro users see the Replay toggle without
a dart-define. Keep `--dart-define=REPLAY_UI=false` as an emergency kill-switch
for builds still pointed at a pre-112a API.

🧱 Scope

- `frontend/lib/features/replay/replay_flags.dart` — default true
- ROADMAP + `PR-112b.md` ship-gate wording

Out of scope: behavior changes, scorecard `until=asOf`, help copy.

✅ Definition of Done

- [x] Default on; `REPLAY_UI=false` still disables bootstrap wiring
- [x] Docs match

🚀 Result

Pro history icon appears on Overview and Market Pulse against a 112a backend.
