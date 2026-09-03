import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/auth/auth_headers.dart';

void main() {
  group('authHeaders', () {
    test('adds Authorization when getSecret returns a value', () {
      final headers = authHeaders(() => 'my-secret');
      expect(headers['Authorization'], 'Bearer my-secret');
    });

    test('omits Authorization when getSecret returns null', () {
      final headers = authHeaders(() => null);
      expect(headers.containsKey('Authorization'), isFalse);
    });

    test('omits Authorization when getSecret is null', () {
      final headers = authHeaders(null);
      expect(headers.containsKey('Authorization'), isFalse);
    });

    test('merges with extra headers', () {
      final headers = authHeaders(
        () => 'my-secret',
        {'Content-Type': 'application/json'},
      );
      expect(headers['Content-Type'], 'application/json');
      expect(headers['Authorization'], 'Bearer my-secret');
    });

    test('omits Authorization for an empty secret', () {
      final headers = authHeaders(() => '');
      expect(headers.containsKey('Authorization'), isFalse);
    });
  });
}
