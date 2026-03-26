import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:pano_chart_frontend/features/notifications/notification_settings_model.dart';
import 'package:pano_chart_frontend/infrastructure/preferences_service.dart';

void main() {
  group('NotificationSettings', () {
    test('defaults enables all categories', () {
      final s = NotificationSettings.defaults();
      expect(s.social, isTrue);
      expect(s.macroHigh, isTrue);
      expect(s.macroModerate, isTrue);
      expect(s.uptrend, isTrue);
      expect(s.downtrend, isTrue);
      expect(s.sideways, isTrue);
      expect(s.setupOfDay, isTrue);
      expect(s.news, isTrue);
      expect(s.uptrendMinDominance, 0.75);
      expect(s.downtrendMinDominance, 0.75);
      expect(s.sidewaysMinDominance, 0.75);
      expect(s.setupMinScore, 0.75);
      expect(s.uptrendTimeframe, '1h');
      expect(s.downtrendTimeframe, '1h');
      expect(s.sidewaysTimeframe, '1h');
      expect(s.setupTimeframe, '1h');
    });

    test('isEnabled maps types correctly', () {
      final s = NotificationSettings(
        social: true,
        macroHigh: false,
        macroModerate: false,
        uptrend: true,
        downtrend: false,
        sideways: false,
        setupOfDay: false,
        news: true,
      );
      expect(s.isEnabled('twitter'), isTrue);
      expect(s.isEnabled('macro'), isFalse);
      // market is enabled if any of uptrend/downtrend/sideways is on.
      expect(s.isEnabled('market'), isTrue);
      expect(s.isEnabled('setup'), isFalse);
      expect(s.isEnabled('news'), isTrue);
      // Unknown type defaults to enabled.
      expect(s.isEnabled('unknown'), isTrue);
    });

    test('market disabled when all three regimes off', () {
      final s = NotificationSettings(
        social: true,
        macroHigh: true,
        macroModerate: true,
        uptrend: false,
        downtrend: false,
        sideways: false,
        setupOfDay: true,
        news: true,
      );
      expect(s.isEnabled('market'), isFalse);
    });

    test('fromPrefs loads persisted values', () async {
      SharedPreferences.setMockInitialValues({
        'settings.notificationsEnabled': false,
        'settings.notify.macroHigh': false,
        'settings.notify.macroModerate': true,
        'settings.notify.uptrend': true,
        'settings.notify.downtrend': false,
        'settings.notify.sideways': true,
        'settings.notify.setupOfDay': false,
        'settings.notify.news': true,
        'settings.notify.uptrendMinDom': 0.80,
        'settings.notify.downtrendMinDom': 0.60,
        'settings.notify.sidewaysMinDom': 0.70,
        'settings.notify.setupMinScore': 0.85,
        'settings.notify.uptrendTf': '15m',
        'settings.notify.downtrendTf': '4h',
        'settings.notify.sidewaysTf': '1d',
        'settings.notify.setupTf': '5m',
      });
      final prefs = await PreferencesService.create();
      final s = NotificationSettings.fromPrefs(prefs);
      expect(s.social, isFalse);
      expect(s.macroHigh, isFalse);
      expect(s.macroModerate, isTrue);
      expect(s.uptrend, isTrue);
      expect(s.downtrend, isFalse);
      expect(s.sideways, isTrue);
      expect(s.setupOfDay, isFalse);
      expect(s.news, isTrue);
      expect(s.uptrendMinDominance, 0.80);
      expect(s.downtrendMinDominance, 0.60);
      expect(s.sidewaysMinDominance, 0.70);
      expect(s.setupMinScore, 0.85);
      expect(s.uptrendTimeframe, '15m');
      expect(s.downtrendTimeframe, '4h');
      expect(s.sidewaysTimeframe, '1d');
      expect(s.setupTimeframe, '5m');
    });

    test('save persists all values', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      final s = NotificationSettings(
        social: false,
        macroHigh: true,
        macroModerate: false,
        uptrend: false,
        downtrend: true,
        sideways: false,
        setupOfDay: true,
        news: false,
        uptrendMinDominance: 0.60,
        downtrendMinDominance: 0.80,
        sidewaysMinDominance: 0.55,
        setupMinScore: 0.90,
        uptrendTimeframe: '15m',
        downtrendTimeframe: '4h',
        sidewaysTimeframe: '1d',
        setupTimeframe: '5m',
      );
      s.save(prefs);
      expect(prefs.notificationsEnabled, isFalse);
      expect(prefs.notifyMacroHigh, isTrue);
      expect(prefs.notifyMacroModerate, isFalse);
      expect(prefs.notifyUptrend, isFalse);
      expect(prefs.notifyDowntrend, isTrue);
      expect(prefs.notifySideways, isFalse);
      expect(prefs.notifySetupOfDay, isTrue);
      expect(prefs.notifyNews, isFalse);
      expect(prefs.uptrendMinDominance, 0.60);
      expect(prefs.downtrendMinDominance, 0.80);
      expect(prefs.sidewaysMinDominance, 0.55);
      expect(prefs.setupMinScore, 0.90);
      expect(prefs.uptrendTimeframe, '15m');
      expect(prefs.downtrendTimeframe, '4h');
      expect(prefs.sidewaysTimeframe, '1d');
      expect(prefs.setupTimeframe, '5m');
    });

    test('fromPrefs uses defaults when no keys set', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      final s = NotificationSettings.fromPrefs(prefs);
      // social defaults to false (existing behavior), others to true.
      expect(s.social, isFalse);
      expect(s.macroHigh, isTrue);
      expect(s.macroModerate, isTrue);
      expect(s.uptrend, isTrue);
      expect(s.downtrend, isTrue);
      expect(s.sideways, isTrue);
      expect(s.setupOfDay, isTrue);
      expect(s.news, isTrue);
      expect(s.uptrendMinDominance, 0.75);
      expect(s.setupMinScore, 0.75);
      expect(s.uptrendTimeframe, '1h');
      expect(s.setupTimeframe, '1h');
    });

    test('toJson produces backend-compatible format', () {
      final s = NotificationSettings(
        social: true,
        macroHigh: false,
        macroModerate: true,
        uptrend: true,
        downtrend: false,
        sideways: true,
        setupOfDay: true,
        news: false,
        uptrendMinDominance: 0.80,
        downtrendMinDominance: 0.65,
        sidewaysMinDominance: 0.70,
        setupMinScore: 0.90,
      );
      final json = s.toJson('user-1');
      expect(json['user_id'], 'user-1');
      expect(json['social'], isTrue);
      expect(json['macro_high'], isFalse);
      expect(json['macro_moderate'], isTrue);
      expect(json['uptrend'], isTrue);
      expect(json['downtrend'], isFalse);
      expect(json['sideways'], isTrue);
      expect(json['setup_of_day'], isTrue);
      expect(json['news'], isFalse);
      expect(json['uptrend_min_dominance'], 0.80);
      expect(json['downtrend_min_dominance'], 0.65);
      expect(json['sideways_min_dominance'], 0.70);
      expect(json['setup_min_score'], 0.90);
      expect(json['uptrend_timeframe'], '1h');
      expect(json['downtrend_timeframe'], '1h');
      expect(json['sideways_timeframe'], '1h');
      expect(json['setup_timeframe'], '1h');
    });

    test('applyFromJson merges server values', () {
      final s = NotificationSettings.defaults();
      s.applyFromJson({
        'social': false,
        'uptrend': false,
        'uptrend_min_dominance': 0.60,
        'setup_min_score': 0.85,
      });
      expect(s.social, isFalse);
      expect(s.uptrend, isFalse);
      expect(s.uptrendMinDominance, 0.60);
      expect(s.setupMinScore, 0.85);
      // Unset fields keep defaults.
      expect(s.downtrend, isTrue);
      expect(s.sidewaysMinDominance, 0.75);
    });

    test('toJson includes timeframes', () {
      final s = NotificationSettings(
        social: true,
        macroHigh: true,
        macroModerate: true,
        uptrend: true,
        downtrend: true,
        sideways: true,
        setupOfDay: true,
        news: true,
        uptrendTimeframe: '15m',
        downtrendTimeframe: '4h',
        sidewaysTimeframe: '1d',
        setupTimeframe: '5m',
      );
      final json = s.toJson('u1');
      expect(json['uptrend_timeframe'], '15m');
      expect(json['downtrend_timeframe'], '4h');
      expect(json['sideways_timeframe'], '1d');
      expect(json['setup_timeframe'], '5m');
    });

    test('applyFromJson merges timeframes', () {
      final s = NotificationSettings.defaults();
      s.applyFromJson({
        'uptrend_timeframe': '15m',
        'setup_timeframe': '4h',
      });
      expect(s.uptrendTimeframe, '15m');
      expect(s.setupTimeframe, '4h');
      // Unset timeframes keep defaults.
      expect(s.downtrendTimeframe, '1h');
      expect(s.sidewaysTimeframe, '1h');
    });
  });
}
