import 'package:flutter/material.dart';

import '../../infrastructure/preferences_service.dart';
import 'notification_settings_model.dart';

/// Callback that builds a screen for a given notification type.
typedef ScreenFactory = Widget Function();

/// Callback for detail deep-links that receive a symbol and optional timeframe.
typedef SymbolTimeframeScreenFactory =
    Widget Function(String symbol, [String? timeframe]);

/// Callback for market deep-links that receive an optional timeframe.
typedef TimeframeScreenFactory = Widget Function(String? timeframe);

/// Routes push notification taps to the correct screen.
///
/// Respects per-category toggle in [PreferencesService].
/// Screens are provided via factory callbacks to avoid coupling to concrete
/// widget constructors.
class NotificationRouter {
  final GlobalKey<NavigatorState> navigatorKey;
  final PreferencesService prefs;
  final ScreenFactory? socialScreen;
  final ScreenFactory? macroScreen;
  final ScreenFactory? newsScreen;
  final TimeframeScreenFactory? marketScreen;
  final SymbolTimeframeScreenFactory? setupScreen;

  NotificationRouter({
    required this.navigatorKey,
    required this.prefs,
    this.socialScreen,
    this.macroScreen,
    this.newsScreen,
    this.marketScreen,
    this.setupScreen,
  });

  /// Routes to the right screen based on [data] payload from FCM.
  void handle(Map<String, dynamic> data) {
    final nav = navigatorKey.currentState;
    if (nav == null) return;

    final type = data['type'] as String?;
    final settings = NotificationSettings.fromPrefs(prefs);

    if (type != null && !settings.isEnabled(type)) return;

    switch (type) {
      case 'twitter':
        if (socialScreen != null) {
          nav.push(MaterialPageRoute(builder: (_) => socialScreen!()));
        }
        break;
      case 'macro':
        if (macroScreen != null) {
          nav.push(MaterialPageRoute(builder: (_) => macroScreen!()));
        }
        break;
      case 'news':
        if (newsScreen != null) {
          nav.push(MaterialPageRoute(builder: (_) => newsScreen!()));
        }
        break;
      case 'market':
        if (marketScreen != null) {
          final timeframe = data['timeframe'] as String?;
          nav.push(MaterialPageRoute(builder: (_) => marketScreen!(timeframe)));
        }
        break;
      case 'setup':
        final symbol = data['symbol'] as String?;
        if (symbol != null && setupScreen != null) {
          final timeframe = data['timeframe'] as String?;
          nav.push(
            MaterialPageRoute(builder: (_) => setupScreen!(symbol, timeframe)),
          );
        }
        break;
      case 'watchlist_transition':
        final symbol = data['symbol'] as String?;
        if (symbol != null && symbol.isNotEmpty && setupScreen != null) {
          final timeframe = data['timeframe'] as String?;
          nav.push(
            MaterialPageRoute(builder: (_) => setupScreen!(symbol, timeframe)),
          );
        }
        break;
    }
  }
}
