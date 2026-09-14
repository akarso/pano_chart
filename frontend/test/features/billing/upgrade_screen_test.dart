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

  // ---- verification-error simulation ----
  //
  // lastVerificationError's real backing field is private to
  // billing_manager.dart, so a subclass can't set it directly — overriding
  // the getter itself is the clean way to drive the exact onChanged-driven
  // sequence a real verification failure produces (clear on a new attempt,
  // set on failure), without needing the full IAP purchase-stream
  // machinery billing_manager_test.dart already exercises separately.

  String? _testVerificationError;

  @override
  String? get lastVerificationError => _testVerificationError;

  /// Simulates what [BillingManager] does internally on a verification
  /// failure: set the error, then notify — see [BillingManager.onChanged].
  void simulateVerificationFailure(String message) {
    _testVerificationError = message;
    onChanged?.call();
  }

  /// Simulates what [BillingManager.purchase] does at the start of every
  /// new attempt: clear any stale error, then notify.
  void simulateNewAttemptStarted() {
    _testVerificationError = null;
    onChanged?.call();
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

    testWidgets(
        'shows a SnackBar for two consecutive failed purchase attempts with the same message',
        (tester) async {
      // CR follow-up regression test: comparing only "is this the same
      // string I already showed" without ever resetting that tracking
      // meant a SECOND failure with the identical fixed message (the
      // realistic case for a persistent server misconfiguration) was
      // silently swallowed — reintroducing the original "purchase
      // completes, nothing visible happens" bug on retry.
      //
      // Explicitly dismisses the first SnackBar (rather than waiting out
      // its real duration, which flutter_test's frame-based pump doesn't
      // reliably fast-forward) and confirms it's actually gone before
      // triggering the second failure — otherwise findsOneWidget after the
      // second failure could pass merely because the *first* SnackBar was
      // still lingering on screen, without proving a second one was ever
      // queued at all (a false-negative this test itself was caught
      // producing while being written, before this explicit-dismiss step
      // was added).
      await tester.pumpWidget(buildApp());
      await tester.pumpAndSettle();

      const message = 'We could not verify your purchase.';

      billing.simulateVerificationFailure(message);
      await tester.pump(); // deliver the SnackBar
      expect(find.text(message), findsOneWidget);

      ScaffoldMessenger.of(tester.element(find.byType(Scaffold)))
          .hideCurrentSnackBar();
      await tester.pumpAndSettle();
      expect(find.text(message), findsNothing,
          reason: 'test setup: the first SnackBar must actually be gone before simulating a second failure');

      // Simulate the user starting a fresh purchase attempt (clears the
      // error), the same sequence BillingManager.purchase() actually
      // drives, then a second failure with the identical message.
      billing.simulateNewAttemptStarted();
      await tester.pump();
      billing.simulateVerificationFailure(message);
      await tester.pump();

      expect(find.text(message), findsOneWidget,
          reason: 'a second failure with the same message must still show a SnackBar');
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
