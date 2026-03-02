import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/detail/trade/trade_links.dart';

void main() {
  group('Exchange enum', () {
    test('label returns human-readable name', () {
      expect(Exchange.binance.label, 'Binance');
      expect(Exchange.mexc.label, 'MEXC');
      expect(Exchange.bybit.label, 'Bybit');
    });

    test('key returns persistence string', () {
      expect(Exchange.binance.key, 'binance');
      expect(Exchange.mexc.key, 'mexc');
      expect(Exchange.bybit.key, 'bybit');
    });

    test('fromKey round-trips all exchanges', () {
      for (final ex in Exchange.values) {
        expect(ExchangeLabel.fromKey(ex.key), ex);
      }
    });

    test('fromKey defaults to binance for unknown key', () {
      expect(ExchangeLabel.fromKey('kraken'), Exchange.binance);
      expect(ExchangeLabel.fromKey(''), Exchange.binance);
    });
  });

  group('TradeSymbolMapper', () {
    test('tradingView prefixes with BINANCE:', () {
      expect(TradeSymbolMapper.tradingView('ETHUSDT'), 'BINANCE:ETHUSDT');
      expect(TradeSymbolMapper.tradingView('btcusdt'), 'BINANCE:BTCUSDT');
    });

    test('binance inserts underscore before quote currency', () {
      expect(TradeSymbolMapper.binance('ETHUSDT'), 'ETH_USDT');
      expect(TradeSymbolMapper.binance('BTCUSDC'), 'BTC_USDC');
      expect(TradeSymbolMapper.binance('BNBBUSD'), 'BNB_BUSD');
    });

    test('mexc inserts underscore (same as binance)', () {
      expect(TradeSymbolMapper.mexc('ETHUSDT'), 'ETH_USDT');
      expect(TradeSymbolMapper.mexc('SOLUSDT'), 'SOL_USDT');
    });

    test('bybit returns uppercase unchanged', () {
      expect(TradeSymbolMapper.bybit('ETHUSDT'), 'ETHUSDT');
      expect(TradeSymbolMapper.bybit('ethusdt'), 'ETHUSDT');
    });

    test('binance handles BTC quote pair', () {
      expect(TradeSymbolMapper.binance('ETHBTC'), 'ETH_BTC');
    });

    test('binance handles ETH quote pair', () {
      expect(TradeSymbolMapper.binance('LINKETH'), 'LINK_ETH');
    });

    test('binance returns symbol as-is when no known quote found', () {
      expect(TradeSymbolMapper.binance('XYZABC'), 'XYZABC');
    });
  });

  group('TimeframeMapper', () {
    test('maps all known timeframes to TradingView intervals', () {
      expect(TimeframeMapper.tradingView('1m'), '1');
      expect(TimeframeMapper.tradingView('5m'), '5');
      expect(TimeframeMapper.tradingView('15m'), '15');
      expect(TimeframeMapper.tradingView('1h'), '60');
      expect(TimeframeMapper.tradingView('4h'), '240');
      expect(TimeframeMapper.tradingView('1d'), 'D');
    });

    test('returns null for unknown timeframe', () {
      expect(TimeframeMapper.tradingView('3d'), isNull);
      expect(TimeframeMapper.tradingView('1w'), isNull);
    });
  });

  group('TradeLinkBuilder', () {
    test('tradingView builds correct URL with symbol and interval', () {
      final uri = TradeLinkBuilder.tradingView('ETHUSDT', '1h');
      expect(uri.scheme, 'https');
      expect(uri.host, 'www.tradingview.com');
      expect(uri.path, '/chart/');
      expect(uri.queryParameters['symbol'], 'BINANCE:ETHUSDT');
      expect(uri.queryParameters['interval'], '60');
    });

    test('tradingView omits interval for unknown timeframe', () {
      final uri = TradeLinkBuilder.tradingView('BTCUSDT', '3d');
      expect(uri.queryParameters.containsKey('interval'), isFalse);
      expect(uri.queryParameters['symbol'], 'BINANCE:BTCUSDT');
    });

    test('exchange builds Binance URL', () {
      final uri = TradeLinkBuilder.exchange('ETHUSDT', Exchange.binance);
      expect(uri.host, 'www.binance.com');
      expect(uri.path, '/trade/ETH_USDT');
    });

    test('exchange builds MEXC URL', () {
      final uri = TradeLinkBuilder.exchange('ETHUSDT', Exchange.mexc);
      expect(uri.host, 'www.mexc.com');
      expect(uri.path, '/exchange/ETH_USDT');
    });

    test('exchange builds Bybit URL', () {
      final uri = TradeLinkBuilder.exchange('ETHUSDT', Exchange.bybit);
      expect(uri.host, 'www.bybit.com');
      expect(uri.path, '/trade/spot/ETHUSDT');
    });
  });
}
