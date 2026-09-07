import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:in_app_purchase/in_app_purchase.dart';
import 'package:in_app_purchase_platform_interface/in_app_purchase_platform_interface.dart';
import 'package:pano_chart_frontend/features/billing/api/subscription_api.dart';
import 'package:pano_chart_frontend/features/billing/billing_manager.dart';

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

class _FakeSubscriptionApi implements SubscriptionApi {
  SubscriptionStatus statusToReturn = SubscriptionStatus.inactive();
  Object? verifyError;
  int verifyCallCount = 0;
  String? lastVerifiedToken;

  @override
  Future<void> verifyPurchase({
    required String provider,
    required String purchaseToken,
    required String userId,
  }) async {
    verifyCallCount++;
    lastVerifiedToken = purchaseToken;
    if (verifyError != null) throw verifyError!;
  }

  @override
  Future<SubscriptionStatus> getStatus(String userId) async => statusToReturn;
}

/// Fake [InAppPurchasePlatform] with a controllable [purchaseStream] and
/// call tracking — see the module doc comment below for why this is needed
/// instead of just subclassing [BillingManager].
class _FakeIapPlatform extends InAppPurchasePlatform {
  final _purchaseController =
      StreamController<List<PurchaseDetails>>.broadcast();
  final List<PurchaseDetails> completedPurchases = [];
  bool available = true;
  bool restoreCalled = false;
  bool buyNonConsumableCalled = false;

  @override
  Stream<List<PurchaseDetails>> get purchaseStream =>
      _purchaseController.stream;

  @override
  Future<bool> isAvailable() async => available;

  @override
  Future<ProductDetailsResponse> queryProductDetails(
      Set<String> identifiers) async {
    return ProductDetailsResponse(
      productDetails: [
        ProductDetails(
          id: identifiers.first,
          title: 'Pano Charts Pro',
          description: 'Full access',
          price: '\$4.99',
          rawPrice: 4.99,
          currencyCode: 'USD',
        ),
      ],
      notFoundIDs: const [],
    );
  }

  @override
  Future<bool> buyNonConsumable({required PurchaseParam purchaseParam}) async {
    buyNonConsumableCalled = true;
    return true;
  }

  @override
  Future<void> completePurchase(PurchaseDetails purchase) async {
    completedPurchases.add(purchase);
  }

  @override
  Future<void> restorePurchases({String? applicationUserName}) async {
    restoreCalled = true;
  }

  /// Simulates the platform delivering a purchase update.
  void push(PurchaseDetails details) => _purchaseController.add([details]);

  void dispose() => _purchaseController.close();
}

PurchaseDetails _purchase({
  PurchaseStatus status = PurchaseStatus.purchased,
  String token = 'server-token',
  bool pendingComplete = false,
}) {
  final details = PurchaseDetails(
    productID: BillingManager.kProductId,
    verificationData: PurchaseVerificationData(
      localVerificationData: 'local',
      serverVerificationData: token,
      source: 'google_play',
    ),
    transactionDate: DateTime.now().millisecondsSinceEpoch.toString(),
    status: status,
  );
  details.pendingCompletePurchase = pendingComplete;
  return details;
}

// ---------------------------------------------------------------------------
// Tests
//
// BillingManager depends on the concrete `InAppPurchase` facade, which can
// only be constructed via its `InAppPurchase.instance` singleton — there's
// no way to build one directly. Its first-ever access in a process tries to
// register the real platform plugin for `defaultTargetPlatform`, which
// `flutter_test` defaults to `TargetPlatform.android`, and that plugin's
// constructor synchronously tries to open a real platform channel — meaning
// simply touching `InAppPurchase.instance` under the default test platform
// throws. Overriding `defaultTargetPlatform` to something the package
// doesn't special-case (`linux`) skips that registration entirely, leaving
// `InAppPurchasePlatform.instance` as whatever we set it to beforehand —
// letting `InAppPurchase.instance`'s methods delegate to our fake platform.
//
// Separately: `purchase()`'s very first non-product-null branch is
// `if (!Platform.isAndroid) return false;` (dart:io Platform — the actual
// host OS, unrelated to the override above) — meaning the "launch a
// purchase" happy path can never be exercised by `flutter test` running on
// a non-Android host (this dev machine included). What CAN be tested, and
// is exactly the money-path orchestration PR-078 is actually about, is
// everything downstream of a purchase update arriving on the stream:
// `_onPurchaseUpdated` and `_verifyAndComplete`, reached via `init()` (which
// has no platform gate) plus the fake stream. `restorePurchases()` also has
// no platform gate and is fully covered directly.
// ---------------------------------------------------------------------------

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  final previousPlatformOverride = debugDefaultTargetPlatformOverride;
  setUpAll(() {
    debugDefaultTargetPlatformOverride = TargetPlatform.linux;
  });
  tearDownAll(() {
    debugDefaultTargetPlatformOverride = previousPlatformOverride;
  });

  late _FakeIapPlatform fakePlatform;
  late _FakeSubscriptionApi fakeApi;
  late BillingManager billing;

  setUp(() {
    fakePlatform = _FakeIapPlatform();
    InAppPurchasePlatform.instance = fakePlatform;
    fakeApi = _FakeSubscriptionApi();
    billing = BillingManager(
      api: fakeApi,
      userId: 'test-user',
      iap: InAppPurchase.instance,
    );
  });

  tearDown(() {
    billing.dispose();
    fakePlatform.dispose();
  });

  group('purchase() guards (platform-independent)', () {
    test('returns false when the product has not loaded yet', () async {
      // init() not called — _product is still null.
      final result = await billing.purchase();
      expect(result, isFalse);
      expect(fakePlatform.buyNonConsumableCalled, isFalse);
    });

    test('returns false on a non-Android host without touching the store',
        () async {
      await billing.init();
      expect(billing.product, isNotNull); // product loaded, so this guard
      // specifically exercises the Platform.isAndroid check, not the
      // product==null one.

      final result = await billing.purchase();

      expect(result, isFalse);
      expect(billing.busy, isFalse,
          reason: 'the platform guard runs before _busy is ever set');
      expect(fakePlatform.buyNonConsumableCalled, isFalse);
    });
  });

  group('_onPurchaseUpdated / _verifyAndComplete (via init() + purchaseStream)',
      () {
    test('TestPurchase_Success_UnlocksAfterServerVerification', () async {
      await billing.init();
      fakeApi.statusToReturn =
          SubscriptionStatus(active: true, expiresAt: DateTime(2099));

      var notifyCount = 0;
      billing.onChanged = () => notifyCount++;

      expect(billing.status.active, isFalse);
      fakePlatform.push(_purchase(token: 'tok-success'));
      await pumpEventQueue();

      expect(fakeApi.verifyCallCount, 1);
      expect(fakeApi.lastVerifiedToken, 'tok-success');
      expect(billing.status.active, isTrue,
          reason: 'unlocks only after server verification completes');
      expect(billing.busy, isFalse);
      expect(notifyCount, greaterThan(0));
    });

    test('TestPurchase_ServerVerificationFails_DoesNotUnlock', () async {
      await billing.init();
      fakeApi.verifyError = Exception('backend rejected the token');
      fakeApi.statusToReturn =
          SubscriptionStatus(active: true, expiresAt: DateTime(2099));

      fakePlatform.push(_purchase());
      await pumpEventQueue();

      expect(fakeApi.verifyCallCount, 1);
      // refreshStatus() is only called after verifyPurchase succeeds, so a
      // failure here must leave status untouched even though the fake API
      // would otherwise report active.
      expect(billing.status.active, isFalse);
      expect(billing.busy, isFalse,
          reason: 'the finally block must still clear busy on failure');
    });

    test('TestPurchase_Restored_ReVerifiesLikeAPurchase', () async {
      await billing.init();
      fakeApi.statusToReturn =
          SubscriptionStatus(active: true, expiresAt: DateTime(2099));

      fakePlatform.push(
          _purchase(status: PurchaseStatus.restored, token: 'tok-restored'));
      await pumpEventQueue();

      expect(fakeApi.verifyCallCount, 1);
      expect(fakeApi.lastVerifiedToken, 'tok-restored');
      expect(billing.status.active, isTrue);
    });

    test('TestPurchase_Error_ClearsBusyWithoutVerifying', () async {
      await billing.init();
      // Put billing in a "busy" state the same way restorePurchases()
      // does — purchase() itself can't reach _busy=true on this platform
      // (see the guard tests above), but the pending/error/canceled
      // branches all need to correctly clear a real in-flight busy flag.
      unawaited(billing.restorePurchases());
      await pumpEventQueue();
      expect(billing.busy, isTrue);

      fakePlatform.push(_purchase(status: PurchaseStatus.error));
      await pumpEventQueue();

      expect(fakeApi.verifyCallCount, 0);
      expect(billing.busy, isFalse);
    });

    test('TestPurchase_Canceled_ReturnsToFreeState', () async {
      await billing.init();
      unawaited(billing.restorePurchases());
      await pumpEventQueue();
      expect(billing.busy, isTrue);

      fakePlatform.push(_purchase(status: PurchaseStatus.canceled));
      await pumpEventQueue();

      expect(fakeApi.verifyCallCount, 0);
      expect(billing.busy, isFalse);
      expect(billing.status.active, isFalse);
    });

    test('TestPurchase_Pending_ShowsPendingState_LeavesBusyUntouched',
        () async {
      await billing.init();
      unawaited(billing.restorePurchases());
      await pumpEventQueue();
      expect(billing.busy, isTrue);

      fakePlatform.push(_purchase(status: PurchaseStatus.pending));
      await pumpEventQueue();

      // Pending purchases don't verify and don't resolve busy either way —
      // the eventual purchased/error/canceled update for the same
      // transaction is what clears it.
      expect(fakeApi.verifyCallCount, 0);
      expect(billing.busy, isTrue);
    });

    test('completes purchases whose pendingCompletePurchase is true',
        () async {
      await billing.init();
      fakeApi.statusToReturn =
          SubscriptionStatus(active: true, expiresAt: DateTime(2099));

      final purchase = _purchase(pendingComplete: true);
      fakePlatform.push(purchase);
      await pumpEventQueue();

      expect(fakePlatform.completedPurchases, hasLength(1));
      expect(fakePlatform.completedPurchases.single.productID,
          BillingManager.kProductId);
    });

    test('does not complete purchases whose pendingCompletePurchase is false',
        () async {
      await billing.init();
      fakePlatform.push(_purchase(pendingComplete: false));
      await pumpEventQueue();

      expect(fakePlatform.completedPurchases, isEmpty);
    });
  });

  group('restorePurchases()', () {
    test('TestRestorePurchases_ReappliesServerVerifiedState', () async {
      await billing.init();
      fakeApi.statusToReturn =
          SubscriptionStatus(active: true, expiresAt: DateTime(2099));

      await billing.restorePurchases();
      expect(fakePlatform.restoreCalled, isTrue);
      expect(billing.busy, isTrue,
          reason: 'restore is in flight until the platform reports a result');

      // Actual restore processing happens via _onPurchaseUpdated — the
      // platform reports restored purchases through the same stream.
      fakePlatform.push(
          _purchase(status: PurchaseStatus.restored, token: 'tok-restore'));
      await pumpEventQueue();

      expect(fakeApi.verifyCallCount, 1);
      expect(fakeApi.lastVerifiedToken, 'tok-restore');
      expect(billing.status.active, isTrue);
      expect(billing.busy, isFalse);
    });
  });
}
