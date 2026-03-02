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

  const ChartIndicatorConfig({
    this.showEmaFast = true,
    this.emaFastPeriod = 20,
    this.showEmaSlow = true,
    this.emaSlowPeriod = 50,
    this.showRsi = true,
    this.rsiPeriod = 14,
    this.showAtr = false,
    this.atrPeriod = 14,
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
          atrPeriod == other.atrPeriod;

  @override
  int get hashCode => Object.hash(
        showEmaFast, emaFastPeriod,
        showEmaSlow, emaSlowPeriod,
        showRsi, rsiPeriod,
        showAtr, atrPeriod,
      );
}
