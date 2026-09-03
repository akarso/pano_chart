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

// ──────────────────────────────────────────────────────────────────────
// Rolling Behavioral Indicators
// ──────────────────────────────────────────────────────────────────────
//
// Ports the backend `application/behavior` engine to Dart as pure
// rolling window functions.  Each returns a List<double> of the same
// length as the input candle arrays (NaN-padded for warmup).
// Values are normalised to 0–100 (like RSI).

/// Holds the four behavioral indicator series.
class BehaviorIndicators {
  final List<double> greed;
  final List<double> fear;
  final List<double> patience;
  final List<double> panic;

  const BehaviorIndicators({
    required this.greed,
    required this.fear,
    required this.patience,
    required this.panic,
  });
}

/// Computes rolling behavioural indicators over a sliding [window].
///
/// At each candle index `i` where `i >= window - 1` we look back [window]
/// candles and derive the same proxy signals the backend uses:
///   - funding proxy (price deviation from SMA)
///   - bullish ratio → imbalance
///   - OI expansion proxy (volume growth)
///   - ATR-based volatility
///   - volume spike
///   - regime proxy (compression / trend / range)
///
/// Then we apply the backend scoring weights and soft-normalise.
/// The result is scaled to 0–100.
BehaviorIndicators computeBehaviorIndicators(
  List<double> closes,
  List<double> highs,
  List<double> lows,
  List<double> volumes,
  int window,
) {
  final n = closes.length;
  final greed = List<double>.filled(n, double.nan);
  final fear = List<double>.filled(n, double.nan);
  final patience = List<double>.filled(n, double.nan);
  final panic = List<double>.filled(n, double.nan);

  if (n < 2 || window < 2) {
    return BehaviorIndicators(
        greed: greed, fear: fear, patience: patience, panic: panic);
  }

  for (var i = window - 1; i < n; i++) {
    final start = i - window + 1;

    // ── Proxy signals within the window ──

    // 1. Funding proxy: (close - SMA) / SMA
    double sum = 0;
    for (var j = start; j <= i; j++) {
      sum += closes[j];
    }
    final sma = sum / window;
    final fundingRaw = sma == 0 ? 0.0 : (closes[i] - sma) / sma;
    // fundingExtremeness: abs(funding) / 0.01, clamped to [0,1]
    final fundingExtremeness = (fundingRaw.abs() / 0.01).clamp(0.0, 1.0);

    // 2. Bullish ratio → imbalance
    int bullish = 0;
    for (var j = start; j <= i; j++) {
      if (closes[j] > (j > 0 ? closes[j - 1] : closes[j])) bullish++;
    }
    final bullRatio = bullish / window;
    final imbalanceVal = ((bullRatio - 0.5).abs() * 2).clamp(0.0, 1.0);

    // 3. OI expansion proxy (volume growth over last 10 or window)
    final oiLen = math.min(10, window);
    final oiStart = i - oiLen + 1;
    final volStart = volumes[oiStart];
    final volEnd = volumes[i];
    double oiExpansion = 0;
    if (volStart > 0) {
      final growth = (volEnd - volStart) / volStart;
      oiExpansion = growth.clamp(0.0, 1.0);
    }

    // 4. Liquidation proximity proxy (nearest high/low to current price)
    final price = closes[i];
    double minDist = double.infinity;
    for (var j = start; j <= i; j++) {
      for (final level in [highs[j], lows[j]]) {
        final dist = (level - price).abs();
        if (dist > 0 && dist < minDist) minDist = dist;
      }
    }
    final liqProx = price == 0
        ? 0.0
        : (1 - (minDist / price) * 10).clamp(0.0, 1.0);

    // 5. Fragility score (weighted composite)
    final fragilityScore =
        (fundingExtremeness * 0.25 +
                oiExpansion * 0.30 +
                imbalanceVal * 0.20 +
                liqProx * 0.25)
            .clamp(0.0, 1.0);

    // 6. ATR volatility
    double atrSum = 0;
    int atrCount = 0;
    for (var j = start + 1; j <= i; j++) {
      final tr = math.max(
          highs[j] - lows[j],
          math.max(
              (highs[j] - closes[j - 1]).abs(),
              (lows[j] - closes[j - 1]).abs()));
      atrSum += tr;
      atrCount++;
    }
    final atr = atrCount > 0 ? atrSum / atrCount : 0.0;
    final volatility = price == 0 ? 0.0 : ((atr / price) * 20).clamp(0.0, 1.0);

    // 7. Volume spike
    double volSum = 0;
    for (var j = start; j <= i; j++) {
      volSum += volumes[j];
    }
    final volAvg = volSum / window;
    double volumeScore = 0;
    if (volAvg > 0) {
      volumeScore = ((volumes[i] / volAvg) - 1.0).clamp(0.0, 1.0);
    }

    // 8. Regime proxy (last 10 candles)
    final regLen = math.min(10, i - start + 1);
    final regStart = i - regLen + 1;
    double regHi = highs[regStart], regLo = lows[regStart];
    for (var j = regStart + 1; j <= i; j++) {
      if (highs[j] > regHi) regHi = highs[j];
      if (lows[j] < regLo) regLo = lows[j];
    }
    final isCompression =
        regLo > 0 && ((regHi - regLo) / regLo) < 0.02;
    final compressionBoost = isCompression ? 0.3 : 0.0;

    // ── Behavioral dimensions (same weights as backend engine.go) ──
    double g =
        (fundingExtremeness * 0.4 + imbalanceVal * 0.4 + oiExpansion * 0.2)
            .clamp(0.0, 1.0);
    double f = (fragilityScore * 0.6 + volatility * 0.4).clamp(0.0, 1.0);
    double p = ((1 - volatility) * 0.5 +
            (1 - volumeScore) * 0.2 +
            compressionBoost)
        .clamp(0.0, 1.0);
    double pa = (fragilityScore * 0.5 + volatility * 0.5).clamp(0.0, 1.0);

    // Soft-normalise (total capped at 1.5, same as backend)
    final total = g + f + p + pa;
    if (total > 1.5) {
      final scale = 1.5 / total;
      g *= scale;
      f *= scale;
      p *= scale;
      pa *= scale;
    }

    // Scale to 0–100
    greed[i] = g * 100;
    fear[i] = f * 100;
    patience[i] = p * 100;
    panic[i] = pa * 100;
  }

  return BehaviorIndicators(
      greed: greed, fear: fear, patience: patience, panic: panic);
}
