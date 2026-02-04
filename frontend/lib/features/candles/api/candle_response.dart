// No extra imports required; keep file minimal to satisfy analyzer.

/// Immutable DTO for a single candle matching PR-011 (timestamp RFC3339, numeric OHLCV).
class CandleDto {
  final DateTime timestamp; // UTC
  final double open;
  final double high;
  final double low;
  final double close;
  final double volume;

  const CandleDto(
      {required this.timestamp,
      required this.open,
      required this.high,
      required this.low,
      required this.close,
      required this.volume});

  factory CandleDto.fromJson(Map<String, dynamic> json) {
    final ts = DateTime.parse(json['timestamp'] as String).toUtc();
    return CandleDto(
      timestamp: ts,
      open: (json['open'] as num).toDouble(),
      high: (json['high'] as num).toDouble(),
      low: (json['low'] as num).toDouble(),
      close: (json['close'] as num).toDouble(),
      volume: (json['volume'] as num).toDouble(),
    );
  }

  Map<String, dynamic> toJson() => {
        'timestamp': timestamp.toUtc().toIso8601String(),
        'open': open,
        'high': high,
        'low': low,
        'close': close,
        'volume': volume,
      };

  @override
  bool operator ==(Object other) {
    return identical(this, other) ||
        (other is CandleDto &&
            timestamp.toUtc() == other.timestamp.toUtc() &&
            open == other.open &&
            high == other.high &&
            low == other.low &&
            close == other.close &&
            volume == other.volume);
  }

  @override
  int get hashCode =>
      Object.hash(timestamp.toUtc(), open, high, low, close, volume);
}

/// Response DTO containing symbol, timeframe and ordered candles.
class CandleSeriesResponse {
  final String symbol;
  final String timeframe;
  final List<CandleDto> candles;

  CandleSeriesResponse({
    required this.symbol,
    required this.timeframe,
    required List<CandleDto> candles,
  }) : candles = List.unmodifiable(candles);

  factory CandleSeriesResponse.fromJson(Map<String, dynamic> json) {
    final cs = (json['candles'] as List<dynamic>)
        .map((e) => CandleDto.fromJson(e as Map<String, dynamic>))
        .toList();
    return CandleSeriesResponse(
        symbol: json['symbol'] as String,
        timeframe: json['timeframe'] as String,
        candles: cs);
  }

  Map<String, dynamic> toJson() => {
        'symbol': symbol,
        'timeframe': timeframe,
        'candles': candles.map((c) => c.toJson()).toList(),
      };
}
