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
