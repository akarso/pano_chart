import 'dart:async';
import 'dart:io';
import 'package:flutter/foundation.dart';
import 'package:in_app_purchase/in_app_purchase.dart';
import '../../core/analytics.dart';
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
  static const String kProductId = 'com.akarso.panocharts';

  /// Current subscription state as reported by the backend.
  SubscriptionStatus _status = SubscriptionStatus.inactive();
  SubscriptionStatus get status => _status;

  /// Product details fetched from the store (null until queried).
  ProductDetails? _product;
  ProductDetails? get product => _product;

  /// True while a purchase or verification is in flight.
  bool _busy = false;
  bool get busy => _busy;

  /// The most recent purchase-verification failure, if any — cleared at
  /// the start of every new purchase/restore attempt and on success.
  ///
  /// Verification runs asynchronously *after* the Play purchase dialog has
  /// already closed and control has returned to the app (via
  /// [_onPurchaseUpdated] → [_verifyAndComplete]), so [purchase]'s own
  /// return value can't carry a verification outcome — it only reports
  /// whether the dialog launched. Before this field existed, a
  /// verification failure was only ever `debugPrint`'d (invisible outside
  /// a debug console), so the user saw the purchase dialog complete
  /// successfully in Play and then... nothing — no error, no confirmation,
  /// same screen. UI layers should surface this (e.g. a SnackBar) whenever
  /// it changes to non-null via [onChanged].
  String? get lastVerificationError => _lastVerificationError;
  String? _lastVerificationError;

  /// True when the user may use all features — either via an active
  /// subscription or an active trial.  When no [TrialManager] is set
  /// (e.g. tests / non-Android), fails closed to `false` — see PR-078.
  ///
  /// In debug builds, [debugOverrideAccess] takes priority when non-null.
  bool get hasFullAccess {
    if (kDebugMode && _debugOverrideAccess != null) {
      return _debugOverrideAccess!;
    }
    return status.active || (_trialManager?.isTrialActive() ?? false);
  }

  // ---- debug helpers (stripped from release builds) ----

  bool? _debugOverrideAccess;

  /// The label of the active debug override, or null if using real state.
  String? get debugOverrideLabel => _debugOverrideLabel;
  String? _debugOverrideLabel;

  /// Forces a specific access level in debug builds. Pass null to reset.
  void debugSetAccess({required bool? fullAccess, String? label}) {
    assert(() {
      _debugOverrideAccess = fullAccess;
      _debugOverrideLabel = fullAccess == null ? null : (label ?? (fullAccess ? 'PRO' : 'FREE'));
      onChanged?.call();
      return true;
    }());
  }

  /// Days left in the free trial (0 when expired or not available).
  int get trialDaysRemaining => _trialManager?.daysRemaining() ?? 0;

  /// True when the user is within the trial window but has no active
  /// subscription yet.
  bool get isTrialMode =>
      !status.active && (_trialManager?.isTrialActive() ?? false);

  /// Callback fired whenever the billing state changes.
  VoidCallback? onChanged;

  StreamSubscription<List<PurchaseDetails>>? _purchaseSub;

  /// Purchase tokens currently being verified — Play Billing often emits
  /// the same token twice in one burst (purchased + restore). Without
  /// this, both hit `/api/payments/verify` and the second can 429.
  final Set<String> _inFlightTokens = {};

  /// PurchaseDetails waiting for acknowledge while a verify for the same
  /// token is already in flight. After a successful verify we ack them.
  final Map<String, List<PurchaseDetails>> _pendingAckByToken = {};

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
    // Reset before the platform guard (not after) so a new attempt clears
    // stale state regardless of how far it gets — including in tests on a
    // non-Android host, where every attempt stops at that guard.
    _lastVerificationError = null;
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

      // Acknowledge error/canceled so Play clears them from the queue.
      // Purchased/restored are acknowledged inside [_verifyAndComplete]
      // only after the backend actually grants access — acknowledging a
      // failed verify used to hide the token from the next restore.
      if (purchase.pendingCompletePurchase &&
          purchase.status != PurchaseStatus.purchased &&
          purchase.status != PurchaseStatus.restored) {
        await _iap.completePurchase(purchase);
      }
    }
  }

  /// Sends the purchase token to the backend for verification,
  /// then refreshes subscription status.
  Future<void> _verifyAndComplete(PurchaseDetails purchase) async {
    final token = purchase.verificationData.serverVerificationData;
    if (token.isEmpty) {
      _lastVerificationError =
          'We could not verify your purchase. Please try "Restore purchases" — '
          'if that doesn\'t work, contact support and we\'ll sort it out.';
      // Play often delivers a batch; an empty entry must not clear busy
      // while a real token is mid-verify.
      if (_inFlightTokens.isEmpty) {
        _busy = false;
      }
      _notify();
      return;
    }
    if (!_inFlightTokens.add(token)) {
      // Another verify for this token is already running. Queue this
      // PurchaseDetails for acknowledge after that verify succeeds —
      // returning without scheduling ack can leave pendingCompletePurchase
      // stuck if Play does not re-emit.
      if (purchase.pendingCompletePurchase) {
        (_pendingAckByToken[token] ??= []).add(purchase);
      }
      return;
    }
    try {
      await _api.verifyPurchase(
        provider: 'google_play',
        purchaseToken: token,
        userId: _userId,
      );
      await refreshStatus();
      if (_status.active) {
        _lastVerificationError = null;
        await _acknowledge(purchase);
        await _acknowledgeQueued(token);
        Analytics().subscriptionStarted(productId: purchase.productID);
      } else {
        // The backend call itself succeeded (no exception), but the
        // subscription still doesn't read as active — e.g. the provider
        // returned a technically-successful-but-not-valid verification
        // result. Surface this too, not just a thrown exception: silently
        // returning to the same "not subscribed" screen with no
        // explanation is the exact failure mode this field exists to fix.
        _lastVerificationError =
            'Purchase completed, but we could not confirm your subscription yet. '
            'Please try "Restore purchases" in a moment, or contact support if this persists.';
        _pendingAckByToken.remove(token);
      }
    } catch (e) {
      debugPrint('[BillingManager] Verification failed: $e');
      // Do NOT treat "some subscription is already active" as proof that
      // *this* token verified. An already-pro user with a bad/new token
      // would otherwise acknowledge a failed delivery to Play and hide the
      // error. Backend idempotency returns 200 for already-processed tokens
      // of this user, so the happy restore path never needs this shortcut.
      await refreshStatus();
      _lastVerificationError = _verificationErrorMessage(e);
      _pendingAckByToken.remove(token);
    } finally {
      _inFlightTokens.remove(token);
      _busy = false;
      _notify();
    }
  }

  Future<void> _acknowledge(PurchaseDetails purchase) async {
    if (purchase.pendingCompletePurchase) {
      await _iap.completePurchase(purchase);
    }
  }

  Future<void> _acknowledgeQueued(String token) async {
    final queued = _pendingAckByToken.remove(token);
    if (queued == null) return;
    for (final details in queued) {
      await _acknowledge(details);
    }
  }

  /// Maps backend failures to something the user can act on. The previous
  /// catch-all hid 429 (rate limit after a few retries) and 401 (stale
  /// device secret) behind the same "could not verify" sentence.
  static String _verificationErrorMessage(Object error) {
    final text = error.toString();
    if (text.contains('(429)')) {
      return 'Too many verification attempts. Please wait a few minutes, '
          'then try "Restore purchases".';
    }
    if (text.contains('(401)')) {
      return 'We could not verify your account. Please restart the app '
          'and try "Restore purchases".';
    }
    return 'We could not verify your purchase. Please try "Restore purchases" — '
        'if that doesn\'t work, contact support and we\'ll sort it out.';
  }

  // ---- restore ----

  /// Restores previous purchases (calls the store's restore API and
  /// re-verifies any found purchase tokens).
  Future<void> restorePurchases() async {
    _busy = true;
    _lastVerificationError = null;
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

  void _notify() {
    onChanged?.call();
  }
}
