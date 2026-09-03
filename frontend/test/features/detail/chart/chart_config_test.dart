import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:pano_chart_frontend/features/detail/chart/chart_config.dart';

void main() {
  group('ChartIndicatorConfig', () {
    test('default values', () {
      const cfg = ChartIndicatorConfig();
      expect(cfg.showEmaFast, true);
      expect(cfg.emaFastPeriod, 20);
      expect(cfg.showEmaSlow, true);
      expect(cfg.emaSlowPeriod, 50);
      expect(cfg.showRsi, true);
      expect(cfg.rsiPeriod, 14);
      expect(cfg.showAtr, false);
      expect(cfg.atrPeriod, 14);
      // Behavioral defaults
      expect(cfg.showBehaviorPanel, false);
      expect(cfg.showGreed, true);
      expect(cfg.showFear, true);
      expect(cfg.showPatience, true);
      expect(cfg.showPanic, true);
      expect(cfg.behaviorWindow, 20);
      // Volatility default
      expect(cfg.showVolatility, false);
    });

    test('copyWith preserves unchanged fields', () {
      const cfg = ChartIndicatorConfig();
      final copy = cfg.copyWith(showAtr: true, atrPeriod: 21);
      expect(copy.showAtr, true);
      expect(copy.atrPeriod, 21);
      // Unchanged
      expect(copy.showEmaFast, true);
      expect(copy.emaFastPeriod, 20);
      expect(copy.showRsi, true);
      expect(copy.showBehaviorPanel, false);
    });

    test('copyWith behavioral fields', () {
      const cfg = ChartIndicatorConfig();
      final copy = cfg.copyWith(
        showBehaviorPanel: true,
        showGreed: false,
        behaviorWindow: 30,
      );
      expect(copy.showBehaviorPanel, true);
      expect(copy.showGreed, false);
      expect(copy.behaviorWindow, 30);
      expect(copy.showFear, true);
    });

    test('equality', () {
      const a = ChartIndicatorConfig();
      const b = ChartIndicatorConfig();
      expect(a, b);
      expect(a.hashCode, b.hashCode);
    });

    test('inequality on any field change', () {
      const base = ChartIndicatorConfig();
      expect(base.copyWith(showEmaFast: false), isNot(base));
      expect(base.copyWith(emaFastPeriod: 10), isNot(base));
      expect(base.copyWith(showRsi: false), isNot(base));
      expect(base.copyWith(showBehaviorPanel: true), isNot(base));
      expect(base.copyWith(behaviorWindow: 30), isNot(base));
      expect(base.copyWith(showVolatility: true), isNot(base));
    });

    test('JSON round-trip', () {
      const original = ChartIndicatorConfig(
        showEmaFast: false,
        emaFastPeriod: 10,
        showEmaSlow: true,
        emaSlowPeriod: 100,
        showRsi: false,
        rsiPeriod: 7,
        showAtr: true,
        atrPeriod: 21,
        showBehaviorPanel: true,
        showGreed: false,
        showFear: true,
        showPatience: false,
        showPanic: true,
        behaviorWindow: 30,
        showVolatility: true,
      );
      final json = original.toJson();
      final restored = ChartIndicatorConfig.fromJson(json);
      expect(restored, original);
    });

    test('fromJson with missing keys returns defaults', () {
      final cfg = ChartIndicatorConfig.fromJson({});
      expect(cfg, const ChartIndicatorConfig());
    });

    group('SharedPreferences persistence', () {
      setUp(() {
        SharedPreferences.setMockInitialValues({});
      });

      test('load returns defaults when nothing saved', () async {
        final prefs = await SharedPreferences.getInstance();
        final cfg = ChartIndicatorConfig.load(prefs);
        expect(cfg, const ChartIndicatorConfig());
      });

      test('save then load round-trips', () async {
        final prefs = await SharedPreferences.getInstance();
        const original = ChartIndicatorConfig(
          showEmaFast: false,
          showAtr: true,
          atrPeriod: 21,
        );
        original.save(prefs);
        final loaded = ChartIndicatorConfig.load(prefs);
        expect(loaded, original);
      });
    });
  });
}
