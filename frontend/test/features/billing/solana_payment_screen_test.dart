import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/billing/api/sol_price_info.dart';
import 'package:pano_chart_frontend/features/billing/api/subscription_api.dart';
import 'package:pano_chart_frontend/features/billing/billing_manager.dart';
import 'package:pano_chart_frontend/features/billing/solana_payment_screen.dart';

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

class _FakeSubscriptionApi implements SubscriptionApi {
  SubscriptionStatus statusToReturn = SubscriptionStatus.inactive();
  bool solPriceShouldFail = false;
  bool verifyShouldActivate = false;

  @override
  Future<void> verifyPurchase({
    required String provider,
    required String purchaseToken,
    required String userId,
  }) async {
    if (verifyShouldActivate) {
      statusToReturn = SubscriptionStatus(
        active: true,
        expiresAt: DateTime.now().add(const Duration(days: 30)),
      );
    }
  }

  @override
  Future<SubscriptionStatus> getStatus(String userId) async {
    return statusToReturn;
  }

  @override
  Future<SolPriceInfo> getSolPrice() async {
    if (solPriceShouldFail) {
      throw Exception('Network error');
    }
    return const SolPriceInfo(
      solPrice: 130.0,
      requiredSOL: 0.038385,
      priceUSD: 4.99,
      wallet: 'TestWallet123abc',
    );
  }
}

class _TestBillingManager extends BillingManager {
  _TestBillingManager({required SubscriptionApi api})
      : super(api: api, userId: 'test_user');

  @override
  Future<void> init() async {}

  @override
  Future<bool> purchase() async => false;

  @override
  Future<void> restorePurchases() async {}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  group('SolanaPaymentScreen', () {
    late _FakeSubscriptionApi fakeApi;
    late _TestBillingManager billing;

    setUp(() {
      fakeApi = _FakeSubscriptionApi();
      billing = _TestBillingManager(api: fakeApi);
    });

    Widget buildScreen() {
      return MaterialApp(
        home: SolanaPaymentScreen(billingManager: billing),
      );
    }

    testWidgets('displays SOL price info after loading', (tester) async {
      await tester.pumpWidget(buildScreen());
      // Initially shows a loading indicator.
      expect(find.byType(CircularProgressIndicator), findsOneWidget);

      // Let the future resolve.
      await tester.pumpAndSettle();

      expect(find.text('€4.99 / month'), findsOneWidget);
      expect(find.textContaining('0.038385 SOL'), findsOneWidget);
      expect(find.text('TestWallet123abc'), findsOneWidget);
    });

    testWidgets('shows error when SOL price fetch fails', (tester) async {
      fakeApi.solPriceShouldFail = true;
      await tester.pumpWidget(buildScreen());
      await tester.pumpAndSettle();

      expect(find.textContaining('Could not fetch SOL price'), findsOneWidget);
    });

    testWidgets('shows error for empty TX signature', (tester) async {
      await tester.pumpWidget(buildScreen());
      await tester.pumpAndSettle();

      // Scroll down to make the Verify button visible.
      await tester.scrollUntilVisible(
        find.text('Verify Payment'),
        200,
        scrollable: find.byType(Scrollable).first,
      );

      // Tap verify without entering anything.
      await tester.tap(find.text('Verify Payment'));
      await tester.pumpAndSettle();

      expect(find.textContaining('Please enter your transaction signature'),
          findsOneWidget);
    });

    testWidgets('shows failure message for invalid tx', (tester) async {
      // verification won't activate the subscription
      fakeApi.verifyShouldActivate = false;
      await tester.pumpWidget(buildScreen());
      await tester.pumpAndSettle();

      // Enter a TX signature.
      await tester.enterText(
          find.byType(TextField), 'badSignature123');

      // Scroll down to make the Verify button visible.
      await tester.scrollUntilVisible(
        find.text('Verify Payment'),
        200,
        scrollable: find.byType(Scrollable).first,
      );

      await tester.tap(find.text('Verify Payment'));
      await tester.pumpAndSettle();

      expect(find.textContaining('Verification failed'), findsOneWidget);
    });
  });

  group('SolPriceInfo', () {
    test('fromJson parses correctly', () {
      final info = SolPriceInfo.fromJson({
        'sol_price': 130.5,
        'required_sol': 0.038238,
        'price_usd': 4.99,
        'wallet': 'ABC123',
      });
      expect(info.solPrice, 130.5);
      expect(info.requiredSOL, 0.038238);
      expect(info.priceUSD, 4.99);
      expect(info.wallet, 'ABC123');
    });

    test('fromJson handles integer values', () {
      final info = SolPriceInfo.fromJson({
        'sol_price': 130,
        'required_sol': 0,
        'price_usd': 5,
        'wallet': 'W',
      });
      expect(info.solPrice, 130.0);
      expect(info.requiredSOL, 0.0);
      expect(info.priceUSD, 5.0);
    });
  });
}
