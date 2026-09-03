import 'dart:math' as math;

/// Formats a price with adaptive precision:
///
/// * `>= 1000`  → no decimals (`12345`)
/// * `>= 1`     → 2–4 decimals for ~5 significant digits (`1.2345`, `99.123`)
/// * `>= 1e-5`  → enough decimals to show 5 significant digits (`0.01385`)
/// * `< 1e-5`   → scientific notation (`1.23e-6`)
///
/// The goal is always ≈ 5 significant digits without unnecessary trailing
/// zeros, switching to exponential when fixed notation would need ≥ 6
/// decimal places.
String formatPrice(double price) {
  if (price.isNaN || price.isInfinite) return price.toString();

  final abs = price.abs();

  if (abs == 0) return '0';

  // Very small values → scientific notation.
  if (abs < 1e-5) {
    return _scientific(price);
  }

  // Determine the number of decimal places needed for ~5 significant digits.
  if (abs >= 1000) return price.toStringAsFixed(0);

  // For values >= 1 we want ~5 significant digits total.
  // digits before decimal = floor(log10(abs)) + 1
  // decimals = max(2, 5 - digitsBefore)
  if (abs >= 1) {
    final digitsBefore = (math.log(abs) / math.ln10).floor() + 1;
    final decimals = math.max(2, 5 - digitsBefore);
    return _trimTrailingZeros(price.toStringAsFixed(decimals), minDecimals: 2);
  }

  // abs < 1: figure out how many leading zeros after the decimal point.
  // e.g. 0.001385 → 3 leading zeros before first significant digit.
  final leadingZeros = -(math.log(abs) / math.ln10).ceil();
  // We want 5 significant digits after the leading zeros.
  final decimals = leadingZeros + 4; // leadingZeros + (5 sig digits - 1)

  // If we'd need >= 6 decimal places, fall back to scientific notation.
  if (decimals >= 6) {
    return _scientific(price);
  }

  return _trimTrailingZeros(price.toStringAsFixed(decimals), minDecimals: 2);
}

/// Formats [value] as e.g. `1.23e-6`.
///
/// Uses [toStringAsExponential] which *always* produces `e` notation,
/// unlike [toStringAsPrecision] which may fall back to fixed form.
String _scientific(double value) {
  // 3 fractional digits in the mantissa → 4 significant digits total.
  final raw = value.toStringAsExponential(3); // e.g. '1.230e-6'
  final parts = raw.split('e');
  final mantissa = _trimTrailingZeros(parts[0], minDecimals: 1);
  return '${mantissa}e${parts[1]}';
}

/// Removes trailing zeros from [s] but keeps at least [minDecimals] after
/// the decimal point.
String _trimTrailingZeros(String s, {int minDecimals = 2}) {
  if (!s.contains('.')) return s;
  final dotIdx = s.indexOf('.');
  final desired = dotIdx + 1 + minDecimals;
  var end = s.length;
  while (end > desired && s[end - 1] == '0') {
    end--;
  }
  return s.substring(0, end);
}
