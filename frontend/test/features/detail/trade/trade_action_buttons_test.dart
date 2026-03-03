import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/detail/trade/exchange_config.dart';
import 'package:pano_chart_frontend/features/detail/trade/trade_action_buttons.dart';

Widget _wrap(Widget child) {
  return MaterialApp(
    home: Scaffold(body: SingleChildScrollView(child: child)),
  );
}

const _testExchanges = [
  ExchangeConfig(id: 'binance', name: 'Binance', urlTemplate: 'https://binance.com/trade/{SYMBOL}_USDT'),
  ExchangeConfig(id: 'mexc', name: 'MEXC', urlTemplate: 'https://mexc.com/trade/{SYMBOL}_USDT'),
  ExchangeConfig(id: 'bybit', name: 'Bybit', urlTemplate: 'https://bybit.com/trade/{SYMBOL}USDT'),
];

void main() {
  group('CustomExchange', () {
    test('buildUrl replaces BTC with base symbol', () {
      const c = CustomExchange(
        name: 'MyDex',
        urlTemplate: 'https://mydex.com/trade/BTC_USDT',
      );
      final uri = c.buildUrl('eth');
      expect(uri.toString(), 'https://mydex.com/trade/ETH_USDT');
    });

    test('buildUrl replaces all BTC occurrences', () {
      const c = CustomExchange(
        name: 'X',
        urlTemplate: 'https://x.com/BTC/trade/BTC',
      );
      final uri = c.buildUrl('sol');
      expect(uri.toString(), 'https://x.com/SOL/trade/SOL');
    });

    test('buildUrl with lowercase input uppercases it', () {
      const c = CustomExchange(
        name: 'X',
        urlTemplate: 'https://x.com/trade/BTC_USDT',
      );
      final uri = c.buildUrl('avax');
      expect(uri.toString(), 'https://x.com/trade/AVAX_USDT');
    });
  });

  group('TradeActionButtons', () {
    testWidgets('renders TradingView and preferred exchange buttons',
        (tester) async {
      await tester.pumpWidget(_wrap(
        TradeActionButtons(
          symbol: 'ETHUSDT',
          timeframe: '1h',
          preferredExchangeId: 'binance',
          exchanges: _testExchanges,
        ),
      ));

      expect(find.text('TradingView'), findsOneWidget);
      expect(find.text('Binance'), findsOneWidget);
      expect(find.byIcon(Icons.show_chart), findsOneWidget);
      expect(find.byIcon(Icons.swap_horiz), findsOneWidget);
    });

    testWidgets('shows "…or choose another" link', (tester) async {
      await tester.pumpWidget(_wrap(
        TradeActionButtons(
          symbol: 'ETHUSDT',
          timeframe: '1h',
          preferredExchangeId: 'binance',
          exchanges: _testExchanges,
        ),
      ));

      expect(find.text('…or choose another'), findsOneWidget);
    });

    testWidgets('preferred exchange label reflects preferredExchangeId',
        (tester) async {
      await tester.pumpWidget(_wrap(
        TradeActionButtons(
          symbol: 'ETHUSDT',
          timeframe: '1h',
          preferredExchangeId: 'mexc',
          exchanges: _testExchanges,
        ),
      ));

      expect(find.text('MEXC'), findsOneWidget);
      expect(find.text('Binance'), findsNothing);
    });

    testWidgets('falls back to first exchange when id not found',
        (tester) async {
      await tester.pumpWidget(_wrap(
        TradeActionButtons(
          symbol: 'ETHUSDT',
          timeframe: '1h',
          preferredExchangeId: 'unknown_exchange',
          exchanges: _testExchanges,
        ),
      ));

      // Falls back to first → Binance
      expect(find.text('Binance'), findsOneWidget);
    });

    testWidgets('shows custom exchange button when provided', (tester) async {
      await tester.pumpWidget(_wrap(
        TradeActionButtons(
          symbol: 'BTCUSDT',
          timeframe: '4h',
          preferredExchangeId: 'binance',
          exchanges: _testExchanges,
          customExchange: const CustomExchange(
            name: 'MyDex',
            urlTemplate: 'https://mydex.com/BTC_USDT',
          ),
        ),
      ));

      expect(find.text('MyDex'), findsOneWidget);
      expect(find.text('edit'), findsOneWidget);
    });

    testWidgets('hides custom exchange section when null', (tester) async {
      await tester.pumpWidget(_wrap(
        TradeActionButtons(
          symbol: 'BTCUSDT',
          timeframe: '4h',
          preferredExchangeId: 'binance',
          exchanges: _testExchanges,
          customExchange: null,
        ),
      ));

      expect(find.text('edit'), findsNothing);
    });

    testWidgets('tapping "…or choose another" opens exchange sheet',
        (tester) async {
      await tester.pumpWidget(_wrap(
        TradeActionButtons(
          symbol: 'ETHUSDT',
          timeframe: '1h',
          preferredExchangeId: 'binance',
          exchanges: _testExchanges,
          onExchangeChanged: (_) {},
          onAddCustom: () {},
        ),
      ));

      await tester.tap(find.text('…or choose another'));
      await tester.pumpAndSettle();

      expect(find.text('Choose Exchange'), findsOneWidget);
      expect(find.text('Binance'), findsWidgets);
      expect(find.text('MEXC'), findsWidgets);
      expect(find.text('Bybit'), findsWidgets);
      expect(find.text('Add your own (url redirect)'), findsOneWidget);
    });

    testWidgets('selecting exchange in sheet fires onExchangeChanged',
        (tester) async {
      String? changed;

      await tester.pumpWidget(_wrap(
        TradeActionButtons(
          symbol: 'ETHUSDT',
          timeframe: '1h',
          preferredExchangeId: 'binance',
          exchanges: _testExchanges,
          onExchangeChanged: (id) => changed = id,
          onAddCustom: () {},
        ),
      ));

      await tester.tap(find.text('…or choose another'));
      await tester.pumpAndSettle();

      // Tap MEXC in the sheet
      await tester.tap(find.text('MEXC').last);
      await tester.pumpAndSettle();

      expect(changed, 'mexc');
    });

    testWidgets('sheet highlights current exchange with radio icon',
        (tester) async {
      await tester.pumpWidget(_wrap(
        TradeActionButtons(
          symbol: 'ETHUSDT',
          timeframe: '1h',
          preferredExchangeId: 'mexc',
          exchanges: _testExchanges,
          onExchangeChanged: (_) {},
          onAddCustom: () {},
        ),
      ));

      await tester.tap(find.text('…or choose another'));
      await tester.pumpAndSettle();

      // MEXC should be checked
      expect(find.byIcon(Icons.radio_button_checked), findsOneWidget);
      // Others unchecked
      expect(find.byIcon(Icons.radio_button_off), findsNWidgets(2));
    });

    testWidgets('tapping "Add your own" in sheet fires onAddCustom',
        (tester) async {
      bool fired = false;

      await tester.pumpWidget(_wrap(
        TradeActionButtons(
          symbol: 'ETHUSDT',
          timeframe: '1h',
          preferredExchangeId: 'binance',
          exchanges: _testExchanges,
          onExchangeChanged: (_) {},
          onAddCustom: () => fired = true,
        ),
      ));

      await tester.tap(find.text('…or choose another'));
      await tester.pumpAndSettle();

      await tester.tap(find.text('Add your own (url redirect)'));
      await tester.pumpAndSettle();

      expect(fired, isTrue);
    });

    testWidgets('tapping "edit" fires onEditCustom', (tester) async {
      bool fired = false;

      await tester.pumpWidget(_wrap(
        TradeActionButtons(
          symbol: 'BTCUSDT',
          timeframe: '1h',
          preferredExchangeId: 'binance',
          exchanges: _testExchanges,
          customExchange: const CustomExchange(
            name: 'MyDex',
            urlTemplate: 'https://mydex.com/BTC',
          ),
          onEditCustom: () => fired = true,
        ),
      ));

      await tester.tap(find.text('edit'));
      await tester.pumpAndSettle();

      expect(fired, isTrue);
    });
  });

  group('showCustomExchangeEditor', () {
    testWidgets('shows dialog with name and URL fields', (tester) async {
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Builder(
            builder: (context) => ElevatedButton(
              onPressed: () => showCustomExchangeEditor(context),
              child: const Text('Open'),
            ),
          ),
        ),
      ));

      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      expect(find.text('Add Custom Exchange'), findsOneWidget);
      expect(find.text('Name'), findsOneWidget);
      expect(find.text('URL'), findsOneWidget);
      expect(find.text('Save'), findsOneWidget);
      expect(find.text('Cancel'), findsOneWidget);
    });

    testWidgets('pre-fills when editing existing', (tester) async {
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Builder(
            builder: (context) => ElevatedButton(
              onPressed: () => showCustomExchangeEditor(
                context,
                existing: const CustomExchange(
                  name: 'OldDex',
                  urlTemplate: 'https://old.com/BTC',
                ),
              ),
              child: const Text('Open'),
            ),
          ),
        ),
      ));

      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      expect(find.text('Edit Custom Exchange'), findsOneWidget);
      expect(find.text('OldDex'), findsOneWidget);
      expect(find.text('https://old.com/BTC'), findsOneWidget);
    });

    testWidgets('cancel returns null', (tester) async {
      CustomExchange? result;

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Builder(
            builder: (context) => ElevatedButton(
              onPressed: () async {
                result = await showCustomExchangeEditor(context);
              },
              child: const Text('Open'),
            ),
          ),
        ),
      ));

      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      await tester.tap(find.text('Cancel'));
      await tester.pumpAndSettle();

      expect(result, isNull);
    });

    testWidgets('save returns CustomExchange with entered values',
        (tester) async {
      CustomExchange? result;

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Builder(
            builder: (context) => ElevatedButton(
              onPressed: () async {
                result = await showCustomExchangeEditor(context);
              },
              child: const Text('Open'),
            ),
          ),
        ),
      ));

      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      // Clear default URL, enter new values
      await tester.enterText(
          find.widgetWithText(TextField, 'Name'), 'NewDex');
      await tester.enterText(
          find.widgetWithText(TextField, 'URL'), 'https://new.com/BTC_USDT');

      await tester.tap(find.text('Save'));
      await tester.pumpAndSettle();

      expect(result, isNotNull);
      expect(result!.name, 'NewDex');
      expect(result!.urlTemplate, 'https://new.com/BTC_USDT');
    });

    testWidgets('save does nothing when name is empty', (tester) async {
      CustomExchange? result;

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Builder(
            builder: (context) => ElevatedButton(
              onPressed: () async {
                result = await showCustomExchangeEditor(context);
              },
              child: const Text('Open'),
            ),
          ),
        ),
      ));

      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      // Leave name empty, enter only URL
      await tester.enterText(
          find.widgetWithText(TextField, 'URL'), 'https://new.com/BTC');

      await tester.tap(find.text('Save'));
      await tester.pumpAndSettle();

      // Dialog should still be showing
      expect(find.text('Save'), findsOneWidget);
      expect(result, isNull);
    });

    testWidgets('shows hint text about BTC placeholder', (tester) async {
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Builder(
            builder: (context) => ElevatedButton(
              onPressed: () => showCustomExchangeEditor(context),
              child: const Text('Open'),
            ),
          ),
        ),
      ));

      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      expect(find.textContaining('BTC'), findsWidgets);
      expect(find.textContaining('placeholder'), findsOneWidget);
    });
  });
}
