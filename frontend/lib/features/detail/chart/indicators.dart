import 'dart:math' as math;

/// Pure-function technical indicator calculations.
///
/// All functions operate on raw `List<double>` inputs and return
/// `List<double>` outputs of the same length (NaN-padded for warm-up).

/// Exponential Moving Average.
///
/// Returns a list of the same length as [values].
/// The first [period]-1 entries are `double.nan` (warm-up period).
List<double> computeEma(List<double> values, int period) {
  if (values.isEmpty || period <= 0) return [];
  final result = List<double>.filled(values.length, double.nan);
  if (period > values.length) return result;

  // Seed with SMA of first `period` values.
  double sum = 0;
  for (var i = 0; i < period; i++) {
    sum += values[i];
  }
  result[period - 1] = sum / period;

  final k = 2.0 / (period + 1);
  for (var i = period; i < values.length; i++) {
    result[i] = values[i] * k + result[i - 1] * (1 - k);
  }
  return result;
}

/// Relative Strength Index (Wilder's smoothing).
///
/// Returns a list of the same length as [closes].
/// The first [period] entries are `double.nan`.
List<double> computeRsi(List<double> closes, int period) {
  if (closes.length < 2 || period <= 0) {
    return List<double>.filled(closes.length, double.nan);
  }
  final result = List<double>.filled(closes.length, double.nan);

  // Compute gains and losses.
  double avgGain = 0;
  double avgLoss = 0;
  for (var i = 1; i <= math.min(period, closes.length - 1); i++) {
    final diff = closes[i] - closes[i - 1];
    if (diff > 0) {
      avgGain += diff;
    } else {
      avgLoss -= diff; // make positive
    }
  }
  if (period >= closes.length) return result;

  avgGain /= period;
  avgLoss /= period;

  result[period] = avgLoss == 0 ? 100.0 : 100.0 - 100.0 / (1.0 + avgGain / avgLoss);

  // Wilder's smoothing.
  for (var i = period + 1; i < closes.length; i++) {
    final diff = closes[i] - closes[i - 1];
    final gain = diff > 0 ? diff : 0.0;
    final loss = diff < 0 ? -diff : 0.0;
    avgGain = (avgGain * (period - 1) + gain) / period;
    avgLoss = (avgLoss * (period - 1) + loss) / period;
    result[i] = avgLoss == 0 ? 100.0 : 100.0 - 100.0 / (1.0 + avgGain / avgLoss);
  }
  return result;
}

/// Average True Range.
///
/// Returns a list of the same length as [highs].
/// The first [period] entries are `double.nan`.
List<double> computeAtr(
  List<double> highs,
  List<double> lows,
  List<double> closes,
  int period,
) {
  final n = highs.length;
  assert(n == lows.length && n == closes.length);
  if (n < 2 || period <= 0) return List<double>.filled(n, double.nan);

  final result = List<double>.filled(n, double.nan);

  // True Range values.
  final tr = List<double>.filled(n, 0);
  tr[0] = highs[0] - lows[0]; // no previous close for bar 0
  for (var i = 1; i < n; i++) {
    final hl = highs[i] - lows[i];
    final hc = (highs[i] - closes[i - 1]).abs();
    final lc = (lows[i] - closes[i - 1]).abs();
    tr[i] = math.max(hl, math.max(hc, lc));
  }

  if (period >= n) return result;

  // Initial ATR is SMA of first `period` TRs (starting from index 1).
  double sum = 0;
  for (var i = 1; i <= period; i++) {
    sum += tr[i];
  }
  result[period] = sum / period;

  // Wilder's smoothing.
  for (var i = period + 1; i < n; i++) {
    result[i] = (result[i - 1] * (period - 1) + tr[i]) / period;
  }
  return result;
}
