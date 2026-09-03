import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:pano_chart_frontend/features/billing/trial_manager.dart';

void main() {
  group('TrialManager', () {
    setUp(() {
      SharedPreferences.setMockInitialValues({});
    });

    test('records install date on first creation', () async {
      final prefs = await SharedPreferences.getInstance();
      final tm = TrialManager(prefs);

      final install = tm.installDate;
      expect(install.isBefore(DateTime.now().toUtc().add(const Duration(seconds: 1))), isTrue);
      expect(install.isAfter(DateTime.now().toUtc().subtract(const Duration(seconds: 5))), isTrue);
    });

    test('does not overwrite install date on subsequent creation', () async {
      final fixed = DateTime.utc(2026, 1, 1);
      SharedPreferences.setMockInitialValues({
        'trial.installDate': fixed.toIso8601String(),
      });
      final prefs = await SharedPreferences.getInstance();
      final tm = TrialManager(prefs);

      expect(tm.installDate, fixed);
    });

    test('trial is active within 14 days', () async {
      final install = DateTime.utc(2026, 3, 1);
      SharedPreferences.setMockInitialValues({
        'trial.installDate': install.toIso8601String(),
      });
      final prefs = await SharedPreferences.getInstance();
      final tm = TrialManager(prefs);

      // Day 0 — active
      expect(tm.isTrialActive(DateTime.utc(2026, 3, 1)), isTrue);
      // Day 7 — mid trial
      expect(tm.isTrialActive(DateTime.utc(2026, 3, 8)), isTrue);
      // Day 13 — last full day (March 14 = 13 days after March 1)
      expect(tm.isTrialActive(DateTime.utc(2026, 3, 14)), isTrue);
    });

    test('trial expires on day 14', () async {
      final install = DateTime.utc(2026, 3, 1);
      SharedPreferences.setMockInitialValues({
        'trial.installDate': install.toIso8601String(),
      });
      final prefs = await SharedPreferences.getInstance();
      final tm = TrialManager(prefs);

      // Day 14 exactly
      expect(tm.isTrialActive(DateTime.utc(2026, 3, 15)), isFalse);
      // Day 15
      expect(tm.isTrialActive(DateTime.utc(2026, 3, 16)), isFalse);
    });

    test('daysRemaining counts down correctly', () async {
      final install = DateTime.utc(2026, 3, 1);
      SharedPreferences.setMockInitialValues({
        'trial.installDate': install.toIso8601String(),
      });
      final prefs = await SharedPreferences.getInstance();
      final tm = TrialManager(prefs);

      expect(tm.daysRemaining(DateTime.utc(2026, 3, 1)), 14);
      expect(tm.daysRemaining(DateTime.utc(2026, 3, 8)), 7);
      expect(tm.daysRemaining(DateTime.utc(2026, 3, 14)), 1);
      expect(tm.daysRemaining(DateTime.utc(2026, 3, 15)), 0);
      expect(tm.daysRemaining(DateTime.utc(2026, 3, 20)), 0);
    });

    test('trialDays constant is 14', () {
      expect(TrialManager.trialDays, 14);
    });
  });
}
