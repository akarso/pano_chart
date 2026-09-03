import 'package:flutter/foundation.dart';

/// Lightweight analytics service for subscription-related events.
///
/// Currently logs to debug console. Replace the implementation with
/// Firebase Analytics, Amplitude, or similar when ready.
class Analytics {
  static final Analytics _instance = Analytics._();
  factory Analytics() => _instance;
  Analytics._();

  void logEvent(String name, [Map<String, String>? params]) {
    debugPrint('[analytics] $name${params != null ? ' $params' : ''}');
  }

  // ── Subscription events ──

  void trialStarted() => logEvent('trial_started');

  void trialExpired() => logEvent('trial_expired');

  void paywallOpened({String? source}) =>
      logEvent('paywall_opened', {'source': source ?? 'unknown'});

  void subscriptionStarted({String? productId}) =>
      logEvent('subscription_started', {'product_id': productId ?? ''});

  void subscriptionCancelled() => logEvent('subscription_cancelled');
}
