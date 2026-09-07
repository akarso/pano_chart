import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/billing/api/subscription_api.dart';
import 'package:pano_chart_frontend/features/billing/billing_manager.dart';
import 'package:pano_chart_frontend/features/billing/capabilities.dart';

class _FakeSubscriptionApi implements SubscriptionApi {
  @override
  Future<void> verifyPurchase({
    required String provider,
    required String purchaseToken,
    required String userId,
  }) async {}

  @override
  Future<SubscriptionStatus> getStatus(String userId) async =>
      SubscriptionStatus.inactive();
}

/// BillingManager that skips IAP connection entirely — only debugSetAccess
/// is used to drive its `hasFullAccess`.
class _TestBillingManager extends BillingManager {
  _TestBillingManager() : super(api: _FakeSubscriptionApi(), userId: 'u');

  @override
  Future<void> init() async {}
}

void main() {
  group('Capabilities.fromBilling', () {
    test('fails closed to the free tier when billingManager is null', () {
      // Regression test for PR-078 CR follow-up: fromBilling(null) used to
      // return .pro() (unconditional access) — locking in the fail-closed
      // fix directly, not just via "existing tests still pass".
      final caps = Capabilities.fromBilling(null);

      expect(caps, isA<Capabilities>());
      expect(caps.isPro, isFalse);
      expect(caps.fullTokenList, isFalse);
      expect(caps.autoRefresh, isFalse);
      expect(caps.futureAxis, isFalse);
      expect(caps.macroExtended, isFalse);
      expect(caps.retailBehavior, isFalse);
      expect(caps.marketPulse, isFalse);
      expect(caps.tokenDiagnostics, isFalse);
      expect(caps.socialFeed, isFalse);
      expect(caps.notificationsFull, isFalse);
    });

    test('returns the pro tier when billingManager reports full access', () {
      final billing = _TestBillingManager()
        ..debugSetAccess(fullAccess: true);

      final caps = Capabilities.fromBilling(billing);

      expect(caps.isPro, isTrue);
      expect(caps.fullTokenList, isTrue);
    });

    test('returns the free tier when billingManager denies full access', () {
      final billing = _TestBillingManager()
        ..debugSetAccess(fullAccess: false);

      final caps = Capabilities.fromBilling(billing);

      expect(caps.isPro, isFalse);
      expect(caps.fullTokenList, isFalse);
    });
  });
}
