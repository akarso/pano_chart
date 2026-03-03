import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/fear_greed/fear_greed_data.dart';
import 'package:pano_chart_frontend/features/fear_greed/fear_greed_dialog.dart';
import 'package:pano_chart_frontend/features/fear_greed/http_fear_greed_api.dart';

// ---- fake API ----

class _FakeFearGreedApi implements FearGreedApi {
  FearGreedData? result;
  bool shouldFail = false;

  @override
  Future<FearGreedData> fetch() async {
    if (shouldFail) throw Exception('network error');
    return result!;
  }
}

void main() {
  group('showFearGreedDialog', () {
    testWidgets('shows value, classification, and date on success',
        (tester) async {
      final api = _FakeFearGreedApi()
        ..result = FearGreedData(
          value: 14,
          classification: 'Extreme Fear',
          timestampUtc: DateTime.utc(2026, 3, 1, 0, 0),
        );

      await tester.pumpWidget(MaterialApp(
        home: Builder(
          builder: (ctx) => ElevatedButton(
            onPressed: () => showFearGreedDialog(ctx, api),
            child: const Text('Open'),
          ),
        ),
      ));

      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      expect(find.text('Fear & Greed Index'), findsOneWidget);
      expect(find.text('14'), findsOneWidget);
      expect(find.text('Extreme Fear'), findsOneWidget);
      expect(find.textContaining('2026-03-01'), findsOneWidget);
      expect(find.text('OK'), findsOneWidget);
    });

    testWidgets('shows error dialog on failure', (tester) async {
      final api = _FakeFearGreedApi()..shouldFail = true;

      await tester.pumpWidget(MaterialApp(
        home: Builder(
          builder: (ctx) => ElevatedButton(
            onPressed: () => showFearGreedDialog(ctx, api),
            child: const Text('Open'),
          ),
        ),
      ));

      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      expect(find.textContaining('Failed to load'), findsOneWidget);
    });

    testWidgets('OK button dismisses dialog', (tester) async {
      final api = _FakeFearGreedApi()
        ..result = FearGreedData(
          value: 50,
          classification: 'Neutral',
          timestampUtc: DateTime.utc(2026, 1, 1),
        );

      await tester.pumpWidget(MaterialApp(
        home: Builder(
          builder: (ctx) => ElevatedButton(
            onPressed: () => showFearGreedDialog(ctx, api),
            child: const Text('Open'),
          ),
        ),
      ));

      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      expect(find.text('50'), findsOneWidget);
      await tester.tap(find.text('OK'));
      await tester.pumpAndSettle();

      expect(find.text('50'), findsNothing);
    });
  });
}
