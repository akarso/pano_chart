import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/core/format_price.dart';

void main() {
  group('formatPrice', () {
    group('large values (>= 1000)', () {
      test('rounds to integer', () {
        expect(formatPrice(12345.67), '12346');
      });

      test('exact thousand', () {
        expect(formatPrice(1000.0), '1000');
      });

      test('very large value', () {
        expect(formatPrice(98765432.1), '98765432');
      });
    });

    group('normal range (1 – 999)', () {
      test('hundreds give ~5 significant digits', () {
        // 123.456 → digitsBefore=3, decimals=max(2,5-3)=2 → "123.46"
        expect(formatPrice(123.456), '123.46');
      });

      test('tens give ~5 significant digits', () {
        // 12.3456 → digitsBefore=2, decimals=max(2,5-2)=3 → "12.346"
        expect(formatPrice(12.3456), '12.346');
      });

      test('single digit gives 4 decimals', () {
        // 1.23456 → digitsBefore=1, decimals=max(2,5-1)=4 → "1.2346"
        expect(formatPrice(1.23456), '1.2346');
      });

      test('trims unnecessary trailing zeros', () {
        // 5.10 → digitsBefore=1, decimals=4 → "5.1000" trimmed to "5.10"
        expect(formatPrice(5.1), '5.10');
      });

      test('keeps at least 2 decimal places', () {
        expect(formatPrice(100.0), '100.00');
      });
    });

    group('sub-dollar (< 1, >= 1e-5)', () {
      test('typical small-cap token price', () {
        // 0.01385 → leadingZeros=2, decimals=2+4=6 → >=6 so scientific
        // Actually: log10(0.01385) ≈ -1.858 → ceil(-1.858) = -1 → leadingZeros = 1
        // Wait, let's recalculate:
        // abs = 0.01385
        // log10(0.01385) ≈ -1.858
        // -(log10(0.01385)) ≈ 1.858
        // ceil(1.858) = 2 → leadingZeros = 2
        // decimals = 2 + 4 = 6 → >=6 → scientific? No, that's wrong.
        // Let me re-read the code:
        // leadingZeros = -(log(abs)/ln10).ceil()
        // log(0.01385)/ln10 = log10(0.01385) ≈ -1.858
        // ceil(-1.858) = -1
        // leadingZeros = -(-1) = 1
        // decimals = 1 + 4 = 5
        // 5 < 6 → toStringAsFixed(5) → "0.01385"
        expect(formatPrice(0.01385), '0.01385');
      });

      test('value needing 4 decimal places', () {
        // 0.1234 → log10(0.1234) ≈ -0.908
        // ceil(-0.908) = 0 → leadingZeros = 0
        // decimals = 0 + 4 = 4
        expect(formatPrice(0.1234), '0.1234');
      });

      test('value around 0.001 uses scientific', () {
        // 0.0012345 → leadingZeros=2, decimals=6 → scientific
        expect(formatPrice(0.0012345), '1.234e-3');
      });

      test('value at boundary 0.00001 uses scientific', () {
        // abs = 1e-5, NOT < 1e-5 → sub-dollar path
        // leadingZeros=5, decimals=9 → scientific
        expect(formatPrice(0.00001), '1.0e-5');
      });
    });

    group('very small values (< 1e-5) – scientific notation', () {
      test('typical micro-cap token', () {
        expect(formatPrice(0.00000123), '1.23e-6');
      });

      test('very tiny value', () {
        final result = formatPrice(0.000000000456);
        expect(result, contains('e'));
        expect(result, '4.56e-10');
      });
    });

    group('edge cases', () {
      test('zero returns "0"', () {
        expect(formatPrice(0.0), '0');
      });

      test('negative zero returns "0"', () {
        expect(formatPrice(-0.0), '0');
      });

      test('NaN returns "NaN"', () {
        expect(formatPrice(double.nan), 'NaN');
      });

      test('infinity returns "Infinity"', () {
        expect(formatPrice(double.infinity), 'Infinity');
      });

      test('negative infinity returns "-Infinity"', () {
        expect(formatPrice(double.negativeInfinity), '-Infinity');
      });

      test('negative large value', () {
        expect(formatPrice(-5000.5), '-5001');
      });

      test('negative normal value', () {
        expect(formatPrice(-42.567), '-42.567');
      });
    });
  });
}
