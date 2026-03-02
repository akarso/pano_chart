import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/detail/trade/trade_action_buttons.dart';
import 'package:pano_chart_frontend/features/detail/trade/trade_links.dart';

Widget _wrap(Widget child) {
  return MaterialApp(
    home: Scaffold(body: child),
  );
}

void main() {
  group('TradeActionButtons', () {
    testWidgets('renders TradingView and exchange buttons', (tester) async {
      await tester.pumpWidget(_wrap(
        TradeActionButtons(
          symbol: 'ETHUSDT',
          timeframe: '1h',
          exchange: Exchange.binance,
        ),
      ));

      expect(find.text('TradingView'), findsOneWidget);
      expect(find.text('Binance'), findsOneWidget);
      expect(find.byIcon(Icons.show_chart), findsOneWidget);
      expect(find.byIcon(Icons.swap_horiz), findsOneWidget);
    });

    testWidgets('exchange label updates when exchange changes', (tester) async {
      await tester.pumpWidget(_wrap(
        TradeActionButtons(
          symbol: 'ETHUSDT',
          timeframe: '1h',
          exchange: Exchange.mexc,
        ),
      ));

      expect(find.text('MEXC'), findsOneWidget);
      expect(find.text('Binance'), findsNothing);
    });

    testWidgets('shows Bybit label for bybit exchange', (tester) async {
      await tester.pumpWidget(_wrap(
        TradeActionButtons(
          symbol: 'BTCUSDT',
          timeframe: '4h',
          exchange: Exchange.bybit,
        ),
      ));

      expect(find.text('Bybit'), findsOneWidget);
    });

    testWidgets('long press on exchange button shows picker', (tester) async {
      Exchange? changed;

      await tester.pumpWidget(_wrap(
        TradeActionButtons(
          symbol: 'ETHUSDT',
          timeframe: '1h',
          exchange: Exchange.binance,
          onExchangeChanged: (ex) => changed = ex,
        ),
      ));

      // Long press the exchange button to open the picker
      await tester.longPress(find.text('Binance'));
      await tester.pumpAndSettle();

      // The bottom sheet should show all exchange options
      expect(find.text('Preferred Exchange'), findsOneWidget);
      expect(find.text('Binance'), findsWidgets); // button + sheet
      expect(find.text('MEXC'), findsOneWidget);
      expect(find.text('Bybit'), findsOneWidget);

      // Tap MEXC
      await tester.tap(find.text('MEXC'));
      await tester.pumpAndSettle();

      expect(changed, Exchange.mexc);
    });
  });

  group('showExchangePicker', () {
    testWidgets('returns null when dismissed', (tester) async {
      Exchange? result;

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Builder(
            builder: (context) => ElevatedButton(
              onPressed: () async {
                result = await showExchangePicker(context, Exchange.binance);
              },
              child: const Text('Pick'),
            ),
          ),
        ),
      ));

      await tester.tap(find.text('Pick'));
      await tester.pumpAndSettle();

      // Dismiss by tapping the scrim
      await tester.tapAt(const Offset(10, 10));
      await tester.pumpAndSettle();

      expect(result, isNull);
    });

    testWidgets('returns selected exchange', (tester) async {
      Exchange? result;

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Builder(
            builder: (context) => ElevatedButton(
              onPressed: () async {
                result = await showExchangePicker(context, Exchange.binance);
              },
              child: const Text('Pick'),
            ),
          ),
        ),
      ));

      await tester.tap(find.text('Pick'));
      await tester.pumpAndSettle();

      await tester.tap(find.text('Bybit'));
      await tester.pumpAndSettle();

      expect(result, Exchange.bybit);
    });

    testWidgets('highlights current exchange', (tester) async {
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Builder(
            builder: (context) => ElevatedButton(
              onPressed: () => showExchangePicker(context, Exchange.mexc),
              child: const Text('Pick'),
            ),
          ),
        ),
      ));

      await tester.tap(find.text('Pick'));
      await tester.pumpAndSettle();

      // The current exchange should have a checked radio icon
      expect(find.byIcon(Icons.radio_button_checked), findsOneWidget);
      // The other two should have unchecked icons
      expect(find.byIcon(Icons.radio_button_off), findsNWidgets(2));
    });
  });
}
