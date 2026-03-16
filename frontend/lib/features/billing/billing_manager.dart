import 'dart:async';
import 'dart:io';
import 'package:flutter/foundation.dart';
import 'package:in_app_purchase/in_app_purchase.dart';
import 'api/sol_price_info.dart';
import 'api/subscription_api.dart';
import 'trial_manager.dart';

/// Manages the full billing lifecycle for Google Play in-app subscriptions:
///
/// * connecting to the billing service
/// * querying product details
/// * launching the purchase flow
/// * receiving purchase results and verifying server-side
/// * restoring previous purchases
///
/// This class is decoupled from UI; the [OverviewWidget] (or any other
/// consumer) interacts through callbacks and state getters.
class BillingManager {
  final SubscriptionApi _api;
  final String _userId;
  final InAppPurchase? _iapOverride;
  final TrialManager? _trialManager;

  /// Lazily resolved IAP instance — avoids accessing the singleton at
  /// construction time which crashes in non-Android test environments.
  InAppPurchase get _iap => _iapOverride ?? InAppPurchase.instance;

  /// The subscription product ID configured in Google Play Console.
  static const String kProductId = 'pano_pro_monthly';

  /// Current subscription state as reported by the backend.
  SubscriptionStatus _status = SubscriptionStatus.inactive();
  SubscriptionStatus get status => _status;

  /// Product details fetched from the store (null until queried).
  ProductDetails? _product;
  ProductDetails? get product => _product;

  /// True while a purchase or verification is in flight.
  bool _busy = false;
  bool get busy => _busy;

  /// True when the user may use all features — either via an active
  /// subscription or an active trial.  When no [TrialManager] is set
  /// (e.g. tests / non-Android), defaults to `true`.
  bool get hasFullAccess =>
      status.active || (_trialManager?.isTrialActive() ?? true);

  /// Days left in the free trial (0 when expired or not available).
  int get trialDaysRemaining => _trialManager?.daysRemaining() ?? 0;

  /// True when the user is within the trial window but has no active
  /// subscription yet.
  bool get isTrialMode =>
      !status.active && (_trialManager?.isTrialActive() ?? false);

  /// Callback fired whenever the billing state changes.
  VoidCallback? onChanged;

  StreamSubscription<List<PurchaseDetails>>? _purchaseSub;

  BillingManager({
    required SubscriptionApi api,
    required String userId,
    InAppPurchase? iap,
    TrialManager? trialManager,
  })  : _api = api,
        _userId = userId,
        _iapOverride = iap,
        _trialManager = trialManager;

  // ---- lifecycle ----

  /// Initialises the billing service, queries product details,
  /// and listens for purchase updates.
  Future<void> init() async {
    final available = await _iap.isAvailable();
    if (!available) {
      debugPrint('[BillingManager] Billing service not available');
      return;
    }

    // Listen to purchase stream.
    _purchaseSub = _iap.purchaseStream.listen(
      _onPurchaseUpdated,
      onError: (e) => debugPrint('[BillingManager] Purchase stream error: $e'),
    );

    // Query product details.
    await _queryProduct();

    // Check backend subscription status.
    await refreshStatus();
  }

  /// Releases resources.
  void dispose() {
    _purchaseSub?.cancel();
    _purchaseSub = null;
  }

  // ---- product query ----

  Future<void> _queryProduct() async {
    final response = await _iap.queryProductDetails({kProductId});
    if (response.productDetails.isNotEmpty) {
      _product = response.productDetails.first;
      debugPrint('[BillingManager] Product loaded: ${_product!.title} — ${_product!.price}');
    } else {
      debugPrint('[BillingManager] Product not found: $kProductId');
      if (response.error != null) {
        debugPrint('[BillingManager] Error: ${response.error}');
      }
    }
    _notify();
  }

  // ---- purchase flow ----

  /// Launches the Google Play purchase dialog.
  /// Returns `true` if the flow was started successfully, `false` otherwise.
  Future<bool> purchase() async {
    if (_product == null || _busy) return false;
    if (!Platform.isAndroid) {
      debugPrint('[BillingManager] Purchase flow only supported on Android');
      return false;
    }

    _busy = true;
    _notify();

    final param = PurchaseParam(productDetails: _product!);
    try {
      final started = await _iap.buyNonConsumable(purchaseParam: param);
      if (!started) {
        _busy = false;
        _notify();
      }
      return started;
    } catch (e) {
      debugPrint('[BillingManager] Purchase error: $e');
      _busy = false;
      _notify();
      return false;
    }
  }

  // ---- purchase update handler ----

  Future<void> _onPurchaseUpdated(List<PurchaseDetails> purchases) async {
    for (final purchase in purchases) {
      switch (purchase.status) {
        case PurchaseStatus.purchased:
        case PurchaseStatus.restored:
          await _verifyAndComplete(purchase);
          break;
        case PurchaseStatus.error:
          debugPrint('[BillingManager] Purchase error: ${purchase.error}');
          _busy = false;
          _notify();
          break;
        case PurchaseStatus.canceled:
          debugPrint('[BillingManager] Purchase canceled');
          _busy = false;
          _notify();
          break;
        case PurchaseStatus.pending:
          debugPrint('[BillingManager] Purchase pending');
          break;
      }

      // Complete pending purchases to acknowledge delivery.
      if (purchase.pendingCompletePurchase) {
        await _iap.completePurchase(purchase);
      }
    }
  }

  /// Sends the purchase token to the backend for verification,
  /// then refreshes subscription status.
  Future<void> _verifyAndComplete(PurchaseDetails purchase) async {
    try {
      await _api.verifyPurchase(
        provider: 'google_play',
        purchaseToken: purchase.verificationData.serverVerificationData,
        userId: _userId,
      );
      await refreshStatus();
    } catch (e) {
      debugPrint('[BillingManager] Verification failed: $e');
    } finally {
      _busy = false;
      _notify();
    }
  }

  // ---- restore ----

  /// Restores previous purchases (calls the store's restore API and
  /// re-verifies any found purchase tokens).
  Future<void> restorePurchases() async {
    _busy = true;
    _notify();
    try {
      await _iap.restorePurchases();
      // Actual restore processing happens via _onPurchaseUpdated.
    } catch (e) {
      debugPrint('[BillingManager] Restore error: $e');
      _busy = false;
      _notify();
    }
  }

  // ---- status ----

  /// Queries the backend for the current subscription status.
  Future<void> refreshStatus() async {
    try {
      _status = await _api.getStatus(_userId);
    } catch (e) {
      debugPrint('[BillingManager] Status check failed: $e');
    }
    _notify();
  }

  // ---- Solana payment ----

  /// Fetches the current SOL price and required amount for a subscription.
  Future<SolPriceInfo> getSolPrice() async {
    return _api.getSolPrice();
  }

  /// Verifies a Solana transaction signature with the backend.
  /// Returns `true` if the subscription was activated, `false` otherwise.
  Future<bool> verifySolanaPayment(String txSignature) async {
    _busy = true;
    _notify();
    try {
      await _api.verifyPurchase(
        provider: 'solana',
        purchaseToken: txSignature,
        userId: _userId,
      );
      await refreshStatus();
      return _status.active;
    } catch (e) {
      debugPrint('[BillingManager] Solana verification failed: $e');
      return false;
    } finally {
      _busy = false;
      _notify();
    }
  }

  void _notify() {
    onChanged?.call();
  }
}
