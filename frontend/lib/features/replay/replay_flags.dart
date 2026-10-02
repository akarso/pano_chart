/// Compile-time gate for the Pro Replay UI (PR-112b).
///
/// Default **on** now that PR-112a `?asOf=` is on `dev`. Bootstrap wires a
/// [ReplayController] when enabled so the Pro history toggle appears.
///
/// Kill-switch for builds that still talk to a pre-112a API:
/// `--dart-define=REPLAY_UI=false`.
const bool kReplayUiEnabled =
    bool.fromEnvironment('REPLAY_UI', defaultValue: true);
