import 'dart:convert';

import 'package:shared_preferences/shared_preferences.dart';

/// Persisted configuration for chart technical indicators.
///
/// Stored as a single JSON blob under `settings.chartIndicators`.
class ChartIndicatorConfig {
  final bool showEmaFast;
  final int emaFastPeriod;
  final bool showEmaSlow;
  final int emaSlowPeriod;
  final bool showRsi;
  final int rsiPeriod;
  final bool showAtr;
  final int atrPeriod;

  // ── Behavioral indicators ──
  final bool showBehaviorPanel;
  final bool showGreed;
  final bool showFear;
  final bool showPatience;
  final bool showPanic;
  final int behaviorWindow;

  // ── Volatility overlay ──
  final bool showVolatility;

  const ChartIndicatorConfig({
    this.showEmaFast = true,
    this.emaFastPeriod = 20,
    this.showEmaSlow = true,
    this.emaSlowPeriod = 50,
    this.showRsi = true,
    this.rsiPeriod = 14,
    this.showAtr = false,
    this.atrPeriod = 14,
    this.showBehaviorPanel = false,
    this.showGreed = true,
    this.showFear = true,
    this.showPatience = true,
    this.showPanic = true,
    this.behaviorWindow = 20,
    this.showVolatility = false,
  });

  ChartIndicatorConfig copyWith({
    bool? showEmaFast,
    int? emaFastPeriod,
    bool? showEmaSlow,
    int? emaSlowPeriod,
    bool? showRsi,
    int? rsiPeriod,
    bool? showAtr,
    int? atrPeriod,
    bool? showBehaviorPanel,
    bool? showGreed,
    bool? showFear,
    bool? showPatience,
    bool? showPanic,
    int? behaviorWindow,
    bool? showVolatility,
  }) {
    return ChartIndicatorConfig(
      showEmaFast: showEmaFast ?? this.showEmaFast,
      emaFastPeriod: emaFastPeriod ?? this.emaFastPeriod,
      showEmaSlow: showEmaSlow ?? this.showEmaSlow,
      emaSlowPeriod: emaSlowPeriod ?? this.emaSlowPeriod,
      showRsi: showRsi ?? this.showRsi,
      rsiPeriod: rsiPeriod ?? this.rsiPeriod,
      showAtr: showAtr ?? this.showAtr,
      atrPeriod: atrPeriod ?? this.atrPeriod,
      showBehaviorPanel: showBehaviorPanel ?? this.showBehaviorPanel,
      showGreed: showGreed ?? this.showGreed,
      showFear: showFear ?? this.showFear,
      showPatience: showPatience ?? this.showPatience,
      showPanic: showPanic ?? this.showPanic,
      behaviorWindow: behaviorWindow ?? this.behaviorWindow,
      showVolatility: showVolatility ?? this.showVolatility,
    );
  }

  Map<String, dynamic> toJson() => {
        'showEmaFast': showEmaFast,
        'emaFastPeriod': emaFastPeriod,
        'showEmaSlow': showEmaSlow,
        'emaSlowPeriod': emaSlowPeriod,
        'showRsi': showRsi,
        'rsiPeriod': rsiPeriod,
        'showAtr': showAtr,
        'atrPeriod': atrPeriod,
        'showBehaviorPanel': showBehaviorPanel,
        'showGreed': showGreed,
        'showFear': showFear,
        'showPatience': showPatience,
        'showPanic': showPanic,
        'behaviorWindow': behaviorWindow,
        'showVolatility': showVolatility,
      };

  factory ChartIndicatorConfig.fromJson(Map<String, dynamic> j) {
    return ChartIndicatorConfig(
      showEmaFast: j['showEmaFast'] as bool? ?? true,
      emaFastPeriod: j['emaFastPeriod'] as int? ?? 20,
      showEmaSlow: j['showEmaSlow'] as bool? ?? true,
      emaSlowPeriod: j['emaSlowPeriod'] as int? ?? 50,
      showRsi: j['showRsi'] as bool? ?? true,
      rsiPeriod: j['rsiPeriod'] as int? ?? 14,
      showAtr: j['showAtr'] as bool? ?? false,
      atrPeriod: j['atrPeriod'] as int? ?? 14,
      showBehaviorPanel: j['showBehaviorPanel'] as bool? ?? false,
      showGreed: j['showGreed'] as bool? ?? true,
      showFear: j['showFear'] as bool? ?? true,
      showPatience: j['showPatience'] as bool? ?? true,
      showPanic: j['showPanic'] as bool? ?? true,
      behaviorWindow: j['behaviorWindow'] as int? ?? 20,
      showVolatility: j['showVolatility'] as bool? ?? false,
    );
  }

  static const _key = 'settings.chartIndicators';

  /// Loads from [SharedPreferences], returning defaults if not stored.
  static ChartIndicatorConfig load(SharedPreferences prefs) {
    final raw = prefs.getString(_key);
    if (raw == null) return const ChartIndicatorConfig();
    try {
      return ChartIndicatorConfig.fromJson(
          jsonDecode(raw) as Map<String, dynamic>);
    } catch (_) {
      return const ChartIndicatorConfig();
    }
  }

  /// Persists to [SharedPreferences].
  Future<void> save(SharedPreferences prefs) async {
    await prefs.setString(_key, jsonEncode(toJson()));
  }

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is ChartIndicatorConfig &&
          showEmaFast == other.showEmaFast &&
          emaFastPeriod == other.emaFastPeriod &&
          showEmaSlow == other.showEmaSlow &&
          emaSlowPeriod == other.emaSlowPeriod &&
          showRsi == other.showRsi &&
          rsiPeriod == other.rsiPeriod &&
          showAtr == other.showAtr &&
          atrPeriod == other.atrPeriod &&
          showBehaviorPanel == other.showBehaviorPanel &&
          showGreed == other.showGreed &&
          showFear == other.showFear &&
          showPatience == other.showPatience &&
          showPanic == other.showPanic &&
          behaviorWindow == other.behaviorWindow &&
          showVolatility == other.showVolatility;

  @override
  int get hashCode => Object.hash(
        showEmaFast, emaFastPeriod,
        showEmaSlow, emaSlowPeriod,
        showRsi, rsiPeriod,
        showAtr, atrPeriod,
        showBehaviorPanel, showGreed,
        showFear, showPatience,
        showPanic, behaviorWindow,
        showVolatility,
      );
}
