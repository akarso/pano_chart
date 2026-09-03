/// Supported external exchanges for deep linking.
enum Exchange {
  binance,
  mexc,
  bybit,
}

extension ExchangeLabel on Exchange {
  String get label {
    switch (this) {
      case Exchange.binance:
        return 'Binance';
      case Exchange.mexc:
        return 'MEXC';
      case Exchange.bybit:
        return 'Bybit';
    }
  }

  /// Persistence key.
  String get key {
    switch (this) {
      case Exchange.binance:
        return 'binance';
      case Exchange.mexc:
        return 'mexc';
      case Exchange.bybit:
        return 'bybit';
    }
  }

  static Exchange fromKey(String key) {
    switch (key) {
      case 'mexc':
        return Exchange.mexc;
      case 'bybit':
        return Exchange.bybit;
      default:
        return Exchange.binance;
    }
  }
}

/// Maps a raw internal symbol (e.g. "ETHUSDT") to the format
/// required by each destination.
class TradeSymbolMapper {
  const TradeSymbolMapper._();

  /// TradingView format: "BINANCE:ETHUSDT"
  static String tradingView(String symbol) {
    // Symbols are already in BASEUSDT format; prefix with exchange.
    return 'BINANCE:${symbol.toUpperCase()}';
  }

  /// Binance web format: "ETH_USDT"
  static String binance(String symbol) => _insertUnderscore(symbol);

  /// MEXC web format: "ETH_USDT"
  static String mexc(String symbol) => _insertUnderscore(symbol);

  /// Bybit web format: "ETHUSDT" (same as internal)
  static String bybit(String symbol) => symbol.toUpperCase();

  /// Splits "ETHUSDT" → "ETH_USDT" by finding the quote currency suffix.
  static String _insertUnderscore(String symbol) {
    final s = symbol.toUpperCase();
    for (final quote in ['USDT', 'USDC', 'BUSD', 'BTC', 'ETH', 'BNB']) {
      if (s.endsWith(quote) && s.length > quote.length) {
        return '${s.substring(0, s.length - quote.length)}_$quote';
      }
    }
    return s; // fallback: return as-is
  }
}

/// Maps internal timeframe strings to TradingView interval values.
class TimeframeMapper {
  const TimeframeMapper._();

  /// Returns the TradingView `interval` query parameter value, or null
  /// if the timeframe has no TradingView equivalent.
  static String? tradingView(String timeframe) {
    switch (timeframe) {
      case '1m':
        return '1';
      case '5m':
        return '5';
      case '15m':
        return '15';
      case '1h':
        return '60';
      case '4h':
        return '240';
      case '1d':
        return 'D';
      default:
        return null;
    }
  }
}

/// Builds deep-link URLs for external apps.
class TradeLinkBuilder {
  const TradeLinkBuilder._();

  /// TradingView chart URL with symbol and optional timeframe.
  static Uri tradingView(String symbol, String timeframe) {
    final tvSymbol = TradeSymbolMapper.tradingView(symbol);
    final interval = TimeframeMapper.tradingView(timeframe);
    final params = <String, String>{'symbol': tvSymbol};
    if (interval != null) params['interval'] = interval;
    return Uri.https('www.tradingview.com', '/chart/', params);
  }

  /// Exchange web URL. Falls back to HTTPS (works whether app is installed
  /// or not — the OS handles the redirect).
  static Uri exchange(String symbol, Exchange exchange) {
    switch (exchange) {
      case Exchange.binance:
        final mapped = TradeSymbolMapper.binance(symbol);
        return Uri.https('www.binance.com', '/trade/$mapped');
      case Exchange.mexc:
        final mapped = TradeSymbolMapper.mexc(symbol);
        return Uri.https('www.mexc.com', '/exchange/$mapped');
      case Exchange.bybit:
        final mapped = TradeSymbolMapper.bybit(symbol);
        return Uri.https('www.bybit.com', '/trade/spot/$mapped');
    }
  }
}
