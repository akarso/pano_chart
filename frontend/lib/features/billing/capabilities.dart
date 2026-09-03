import 'billing_manager.dart';

/// Capability-based feature gating derived from subscription state.
///
/// Replaces the boolean `_isProUser` pattern with a structured model
/// that maps subscription status to individual feature flags.
class Capabilities {
  final bool fullTokenList;
  final bool autoRefresh;
  final bool futureAxis;
  final bool macroExtended;
  final bool retailBehavior;
  final bool marketPulse;
  final bool tokenDiagnostics;
  final bool socialFeed;
  final bool notificationsFull;

  const Capabilities({
    required this.fullTokenList,
    required this.autoRefresh,
    required this.futureAxis,
    required this.macroExtended,
    required this.retailBehavior,
    required this.marketPulse,
    required this.tokenDiagnostics,
    required this.socialFeed,
    required this.notificationsFull,
  });

  /// All features unlocked — used for TRIAL / ACTIVE / GRACE states.
  const Capabilities.pro()
      : fullTokenList = true,
        autoRefresh = true,
        futureAxis = true,
        macroExtended = true,
        retailBehavior = true,
        marketPulse = true,
        tokenDiagnostics = true,
        socialFeed = true,
        notificationsFull = true;

  /// Restricted feature set — used for FREE / EXPIRED states.
  const Capabilities.free()
      : fullTokenList = false,
        autoRefresh = false,
        futureAxis = false,
        macroExtended = false,
        retailBehavior = false,
        marketPulse = false,
        tokenDiagnostics = false,
        socialFeed = false,
        notificationsFull = false;

  /// Derives capabilities from the current billing state.
  factory Capabilities.fromBilling(BillingManager? billing) {
    if (billing == null || billing.hasFullAccess) {
      return const Capabilities.pro();
    }
    return const Capabilities.free();
  }

  /// Whether the user has full (pro) access.
  bool get isPro => fullTokenList;
}
