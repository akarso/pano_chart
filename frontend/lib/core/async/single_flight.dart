/// Wraps [fn] so that concurrent calls made while one invocation is still
/// running share that single in-flight call instead of each starting their
/// own.
///
/// Built for coordinating a shared credential refresh: several requests can
/// fail with 401 around the same moment (e.g. a burst of calls fired at app
/// startup before a device secret has been claimed), and each one's retry
/// logic independently wants to "re-claim the credential" — without this,
/// they'd race, and losing callers would see their own claim attempt
/// rejected (already-claimed) while the winner's result hadn't been
/// persisted yet, leaving them stuck on a stale/absent secret. With this
/// wrapper, every concurrent caller awaits the exact same underlying call
/// and observes its outcome once it completes.
Future<void> Function() singleFlight(Future<void> Function() fn) {
  Future<void>? inFlight;
  return () {
    return inFlight ??= fn().whenComplete(() => inFlight = null);
  };
}
