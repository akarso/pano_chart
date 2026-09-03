import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:pano_chart_frontend/infrastructure/preferences_service.dart';

void main() {
  group('PreferencesService.userId', () {
    setUp(() {
      SharedPreferences.setMockInitialValues({});
    });

    test('generates a valid v4 UUID on first access', () async {
      final prefs = await SharedPreferences.getInstance();
      final svc = PreferencesService(prefs);

      final id = svc.userId;

      // UUID v4 format: 8-4-4-4-12 hex chars
      final uuidRegex = RegExp(
        r'^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$',
      );
      expect(uuidRegex.hasMatch(id), isTrue, reason: 'Expected valid v4 UUID, got: $id');
    });

    test('returns same id on subsequent accesses', () async {
      final prefs = await SharedPreferences.getInstance();
      final svc = PreferencesService(prefs);

      final id1 = svc.userId;
      final id2 = svc.userId;

      expect(id1, id2);
    });

    test('persists across PreferencesService instances', () async {
      final prefs = await SharedPreferences.getInstance();
      final svc1 = PreferencesService(prefs);
      final id1 = svc1.userId;

      // Second instance using the same SharedPreferences.
      final svc2 = PreferencesService(prefs);
      expect(svc2.userId, id1);
    });
  });
}
