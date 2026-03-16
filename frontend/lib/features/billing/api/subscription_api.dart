import 'sol_price_info.dart';

/// Port defining the subscription verification service interface.
///
/// Decouples the billing layer from the concrete HTTP implementation.
abstract class SubscriptionApi {
  /// Sends a verified purchase token to the backend for server-side
  /// verification and subscription activation.
  Future<void> verifyPurchase({
    required String provider,
    required String purchaseToken,
    required String userId,
  });

  /// Queries the backend for the current subscription status of [userId].
  Future<SubscriptionStatus> getStatus(String userId);

  /// Fetches the current SOL price and required amount for a subscription.
  Future<SolPriceInfo> getSolPrice();
}

/// Immutable value object describing the user's subscription state.
class SubscriptionStatus {
  final bool active;
  final DateTime? expiresAt;

  const SubscriptionStatus({required this.active, this.expiresAt});

  factory SubscriptionStatus.inactive() =>
      const SubscriptionStatus(active: false);

  @override
  String toString() =>
      'SubscriptionStatus(active=$active, expiresAt=$expiresAt)';
}
