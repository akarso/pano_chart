/// Compile-time gate for the Pro Replay UI (PR-112b).
///
/// When false, [bootstrapApp] does not wire a [ReplayController], so the
/// Replay toggle stays hidden. Flip to true only once PR-112a `?asOf=` is
/// live on the API this build talks to — otherwise the banner would label
/// live payloads as replay.
///
/// Override locally with `--dart-define=REPLAY_UI=true`.
const bool kReplayUiEnabled =
    bool.fromEnvironment('REPLAY_UI', defaultValue: false);
