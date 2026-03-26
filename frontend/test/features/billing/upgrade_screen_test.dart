import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:pano_chart_frontend/features/billing/api/subscription_api.dart';
import 'package:pano_chart_frontend/features/billing/billing_manager.dart';
import 'package:pano_chart_frontend/features/billing/trial_manager.dart';
import 'package:pano_chart_frontend/features/billing/upgrade_screen.dart';

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

class _FakeSubscriptionApi implements SubscriptionApi {
  SubscriptionStatus statusToReturn = SubscriptionStatus.inactive();
  int verifyCallCount = 0;

  @override
  Future<void> verifyPurchase({
    required String provider,
    required String purchaseToken,
    required String userId,
  }) async {
    verifyCallCount++;
  }

  @override
  Future<SubscriptionStatus> getStatus(String userId) async {
    return statusToReturn;
  }
}

/// Minimal BillingManager that doesn't touch InAppPurchase at all.
/// We override init() to be a no-op so tests run without the billing SDK.
class _TestBillingManager extends BillingManager {
  _TestBillingManager({
    required SubscriptionApi api,
    TrialManager? trialManager,
  }) : super(api: api, userId: 'test_user', trialManager: trialManager);

  @override
  Future<void> init() async {
    // No-op: skip IAP connection in tests.
  }

  @override
  Future<bool> purchase() async => false;

  @override
  Future<void> restorePurchases() async {
    // No-op in tests.
  }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  group('UpgradeScreen', () {
    late _FakeSubscriptionApi fakeApi;
    late _TestBillingManager billing;

    setUp(() {
      fakeApi = _FakeSubscriptionApi();
      billing = _TestBillingManager(api: fakeApi);
    });

    Widget buildApp() {
      return MaterialApp(
        home: UpgradeScreen(billingManager: billing),
      );
    }

    testWidgets('shows subscribe UI when not subscribed', (tester) async {
      await tester.pumpWidget(buildApp());
      await tester.pumpAndSettle();

      expect(find.text('Upgrade to Pro'), findsWidgets);
      expect(find.text('Subscribe'), findsOneWidget);
      expect(find.text('Restore purchases'), findsOneWidget);
      expect(find.text('Manage subscription'), findsNothing);
    });

    testWidgets('shows manage UI when subscribed', (tester) async {
      fakeApi.statusToReturn = SubscriptionStatus(
        active: true,
        expiresAt: DateTime(2025, 12, 31),
      );
      // Pre-load the active status into the billing manager.
      await billing.refreshStatus();

      await tester.pumpWidget(buildApp());
      await tester.pumpAndSettle();

      expect(find.text('You are subscribed to Pro'), findsOneWidget);
      expect(find.text('Manage subscription'), findsOneWidget);
      expect(find.text('Subscribe'), findsNothing);
      expect(find.text('Expires: 2025-12-31'), findsOneWidget);
    });

    testWidgets('shows snackbar when purchase fails', (tester) async {
      await tester.pumpWidget(buildApp());
      await tester.pumpAndSettle();

      await tester.tap(find.text('Subscribe'));
      await tester.pumpAndSettle();

      expect(find.text('Could not start purchase flow'), findsOneWidget);
    });

    testWidgets('restore shows "no subscription" snackbar', (tester) async {
      await tester.pumpWidget(buildApp());
      await tester.pumpAndSettle();

      await tester.tap(find.text('Restore purchases'));
      await tester.pumpAndSettle();

      expect(find.text('No previous subscription found'), findsOneWidget);
    });

    testWidgets('shows trial days remaining when trial active', (tester) async {
      // Install date 10 days ago → 4 days remaining.
      final install = DateTime.now().toUtc().subtract(const Duration(days: 10));
      SharedPreferences.setMockInitialValues({
        'trial.installDate': install.toIso8601String(),
      });
      final prefs = await SharedPreferences.getInstance();
      final trial = TrialManager(prefs);

      fakeApi = _FakeSubscriptionApi();
      billing = _TestBillingManager(api: fakeApi, trialManager: trial);

      await tester.pumpWidget(buildApp());
      await tester.pumpAndSettle();

      expect(find.textContaining('days left in free trial'), findsOneWidget);
    });

    testWidgets('shows trial expired message when trial over', (tester) async {
      // Install date 20 days ago → expired.
      final install = DateTime.now().toUtc().subtract(const Duration(days: 20));
      SharedPreferences.setMockInitialValues({
        'trial.installDate': install.toIso8601String(),
      });
      final prefs = await SharedPreferences.getInstance();
      final trial = TrialManager(prefs);

      fakeApi = _FakeSubscriptionApi();
      billing = _TestBillingManager(api: fakeApi, trialManager: trial);

      await tester.pumpWidget(buildApp());
      await tester.pumpAndSettle();

      expect(find.text('Trial Expired'), findsOneWidget);
      expect(find.textContaining('14-day free trial has ended'), findsOneWidget);
    });
  });
}
