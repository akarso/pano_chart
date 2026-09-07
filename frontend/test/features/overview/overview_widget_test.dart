import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/core/app_lifecycle_manager.dart';
import 'package:pano_chart_frontend/features/billing/api/subscription_api.dart';
import 'package:pano_chart_frontend/features/billing/billing_manager.dart';
import 'package:pano_chart_frontend/features/candles/application/get_candle_series.dart';
import 'package:pano_chart_frontend/features/candles/application/get_candle_series_input.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/features/overview/overview_widget.dart';
import 'package:pano_chart_frontend/features/overview/overview_view_model.dart';
import 'package:pano_chart_frontend/features/overview/get_overview.dart';
import 'package:pano_chart_frontend/features/overview/overview_state.dart';

/// Minimal SubscriptionApi fake — nothing in these tests actually calls it,
/// BillingManager just requires one to construct.
class _FakeSubscriptionApi implements SubscriptionApi {
  @override
  Future<void> verifyPurchase({
    required String provider,
    required String purchaseToken,
    required String userId,
  }) async {}

  @override
  Future<SubscriptionStatus> getStatus(String userId) async =>
      SubscriptionStatus.inactive();
}

/// BillingManager that skips IAP connection entirely — tests drive access
/// level directly via debugSetAccess instead of a real purchase/trial flow.
class _TestBillingManager extends BillingManager {
  _TestBillingManager()
      : super(api: _FakeSubscriptionApi(), userId: 'test_user');

  @override
  Future<void> init() async {}
}

class _FakeGetOverview extends GetOverview {
  final Duration delay;
  final OverviewResult result;
  final List<int> pageCalls = [];

  _FakeGetOverview({this.delay = Duration.zero, required this.result});

  @override
  Future<OverviewResult> call({
    required String timeframe,
    required int page,
    required String sort,
    String? snapshot,
    String sidewaysAlgo = 'v1',
    List<String> symbols = const [],
  }) async {
    pageCalls.add(page);
    if (delay != Duration.zero) await Future.delayed(delay);
    return result;
  }
}

class _FakeGetCandleSeries implements GetCandleSeries {
  @override
  Future<CandleSeriesResponse> execute(GetCandleSeriesInput input) async {
    return CandleSeriesResponse(
      symbol: input.symbol,
      timeframe: input.timeframe,
      candles: [],
    );
  }
}

Widget _wrap(Widget w) => MaterialApp(home: Scaffold(body: w));

void main() {
  testWidgets('OverviewScreen_showsLoadingState', (WidgetTester tester) async {
    final getOverview = _FakeGetOverview(
      delay: const Duration(milliseconds: 200),
      result: const OverviewResult(items: [], hasMore: false),
    );
    final vm = OverviewViewModel(getOverview);

    final widget = OverviewWidget(
      viewModel: vm,
      getCandleSeries: _FakeGetCandleSeries(),
    );

    await tester.pumpWidget(_wrap(widget));
    expect(find.byType(CircularProgressIndicator), findsOneWidget);
    await tester.pumpAndSettle();
  });

  testWidgets('OverviewScreen_rendersList', (WidgetTester tester) async {
    final items = [
      const OverviewItem(
        symbol: 'BTCUSDT',
        totalScore: 2.75,
        sparkline: [100.0, 105.0, 110.0],
      ),
      const OverviewItem(
        symbol: 'ETHUSD',
        totalScore: -1.5,
        sparkline: [200.0, 195.0, 190.0],
      ),
    ];

    final getOverview = _FakeGetOverview(
      result: OverviewResult(items: items, hasMore: false),
    );
    final vm = OverviewViewModel(getOverview);

    final widget = OverviewWidget(
      viewModel: vm,
      getCandleSeries: _FakeGetCandleSeries(),
    );

    await tester.pumpWidget(_wrap(widget));
    await tester.pumpAndSettle();

    expect(find.textContaining('BTC'), findsOneWidget);
    expect(find.textContaining('ETH'), findsOneWidget);
  });

  testWidgets('OverviewScreen_handlesEmptySparkline',
      (WidgetTester tester) async {
    final items = [
      const OverviewItem(symbol: 'BTCUSDT'),
    ];

    final getOverview = _FakeGetOverview(
      result: OverviewResult(items: items, hasMore: false),
    );
    final vm = OverviewViewModel(getOverview);

    final widget = OverviewWidget(
      viewModel: vm,
      getCandleSeries: _FakeGetCandleSeries(),
    );

    await tester.pumpWidget(_wrap(widget));
    await tester.pumpAndSettle();

    expect(find.text('No data'), findsOneWidget);
  });

  group('weak-signal banner', () {
    testWidgets('shows disclaimer when sort-relevant scores are below threshold',
        (WidgetTester tester) async {
      final items = List.generate(
        5,
        (i) => OverviewItem(
          symbol: 'SYM${i}USDT',
          totalScore: 0.15,
          sidewaysScore: 0.15, // below 0.30 threshold for sideways sort
          sparkline: const [100.0, 101.0],
        ),
      );

      final vm = OverviewViewModel(_FakeGetOverview(
        result: OverviewResult(items: items, hasMore: false),
      ));
      vm.changeSortSilent('sideways');

      await tester.pumpWidget(_wrap(
        OverviewWidget(viewModel: vm, getCandleSeries: _FakeGetCandleSeries()),
      ));
      await tester.pumpAndSettle();

      expect(
        find.textContaining('weakly represented'),
        findsOneWidget,
      );
    });

    testWidgets('hides disclaimer when sort-relevant scores are above threshold',
        (WidgetTester tester) async {
      final items = List.generate(
        5,
        (i) => OverviewItem(
          symbol: 'SYM${i}USDT',
          totalScore: 0.80,
          sidewaysScore: 0.80, // above threshold
          sparkline: const [100.0, 101.0],
        ),
      );

      final vm = OverviewViewModel(_FakeGetOverview(
        result: OverviewResult(items: items, hasMore: false),
      ));
      vm.changeSortSilent('sideways');

      await tester.pumpWidget(_wrap(
        OverviewWidget(viewModel: vm, getCandleSeries: _FakeGetCandleSeries()),
      ));
      await tester.pumpAndSettle();

      expect(
        find.textContaining('weakly represented'),
        findsNothing,
      );
    });

    testWidgets('checks trend score when sorting by trend',
        (WidgetTester tester) async {
      // High totalScore but low trendScore → banner should show.
      final items = List.generate(
        5,
        (i) => OverviewItem(
          symbol: 'SYM${i}USDT',
          totalScore: 0.80,
          sidewaysScore: 0.75,
          trendScore: 0.10, // weak trend despite high total
          sparkline: const [100.0, 101.0],
        ),
      );

      final vm = OverviewViewModel(_FakeGetOverview(
        result: OverviewResult(items: items, hasMore: false),
      ));
      vm.changeSortSilent('trend');

      await tester.pumpWidget(_wrap(
        OverviewWidget(viewModel: vm, getCandleSeries: _FakeGetCandleSeries()),
      ));
      await tester.pumpAndSettle();

      expect(
        find.textContaining('weakly represented'),
        findsOneWidget,
      );
    });

    testWidgets('suppressed for volume/gain/losers sorts',
        (WidgetTester tester) async {
      final items = List.generate(
        5,
        (i) => OverviewItem(
          symbol: 'SYM${i}USDT',
          totalScore: 0.10, // very weak
          sparkline: const [100.0, 101.0],
        ),
      );

      for (final sort in ['volume', 'gain', 'losers']) {
        final vm = OverviewViewModel(_FakeGetOverview(
          result: OverviewResult(items: items, hasMore: false),
        ));
        vm.changeSortSilent(sort);

        await tester.pumpWidget(_wrap(
          OverviewWidget(
              viewModel: vm, getCandleSeries: _FakeGetCandleSeries()),
        ));
        await tester.pumpAndSettle();

        expect(
          find.textContaining('weakly represented'),
          findsNothing,
          reason: 'banner should be hidden for sort=$sort',
        );
      }
    });
  });

  group('free-tier upgrade banner', () {
    List<OverviewItem> manyItems(int n) => List.generate(
          n,
          (i) => OverviewItem(
            symbol: 'SYM${i}USDT',
            totalScore: 1.0,
            sparkline: const [100.0, 101.0],
          ),
        );

    testWidgets('free-tier user with >15 tokens sees the upgrade banner',
        (WidgetTester tester) async {
      final vm = OverviewViewModel(_FakeGetOverview(
        result: OverviewResult(items: manyItems(20), hasMore: false),
      ));
      final billing = _TestBillingManager()..debugSetAccess(fullAccess: false);

      await tester.pumpWidget(_wrap(
        OverviewWidget(
          viewModel: vm,
          getCandleSeries: _FakeGetCandleSeries(),
          billingManager: billing,
        ),
      ));
      await tester.pumpAndSettle();

      // The banner tile is the 16th grid cell (15 capped items + 1) — below
      // the fold in the default test viewport, so the lazily-built
      // GridView.builder won't have built it yet without scrolling there.
      await tester.scrollUntilVisible(
        find.textContaining('more tokens with Pro'),
        300.0,
        scrollable: find.byType(Scrollable),
      );

      expect(find.textContaining('more tokens with Pro'), findsOneWidget);
      expect(find.text('+5 more tokens with Pro'), findsOneWidget);
    });

    testWidgets('pro user with >15 tokens does not see the upgrade banner',
        (WidgetTester tester) async {
      final vm = OverviewViewModel(_FakeGetOverview(
        result: OverviewResult(items: manyItems(20), hasMore: false),
      ));
      final billing = _TestBillingManager()..debugSetAccess(fullAccess: true);

      await tester.pumpWidget(_wrap(
        OverviewWidget(
          viewModel: vm,
          getCandleSeries: _FakeGetCandleSeries(),
          billingManager: billing,
        ),
      ));
      await tester.pumpAndSettle();

      // Scroll all the way down first — otherwise a wrongly-inserted
      // banner tile near the end of a 20-item grid would sit below the
      // fold, unbuilt by the lazy GridView, and findsNothing would pass
      // for the wrong reason (never looked) rather than because the
      // banner is genuinely absent. Confirmed this catches a real
      // regression: temporarily dropping the entitlement check from the
      // cap condition still passed the assertion without this scroll.
      await tester.scrollUntilVisible(
        find.text('SYM19'),
        300.0,
        scrollable: find.byType(Scrollable),
      );

      expect(find.textContaining('more tokens with Pro'), findsNothing);
    });

    testWidgets('free-tier user with <=15 tokens does not see the banner',
        (WidgetTester tester) async {
      final vm = OverviewViewModel(_FakeGetOverview(
        result: OverviewResult(items: manyItems(10), hasMore: false),
      ));
      final billing = _TestBillingManager()..debugSetAccess(fullAccess: false);

      await tester.pumpWidget(_wrap(
        OverviewWidget(
          viewModel: vm,
          getCandleSeries: _FakeGetCandleSeries(),
          billingManager: billing,
        ),
      ));
      await tester.pumpAndSettle();

      expect(find.textContaining('more tokens with Pro'), findsNothing);
    });

    testWidgets(
        'free-tier user scrolling to the cap does not trigger pagination',
        (WidgetTester tester) async {
      final getOverview = _FakeGetOverview(
        result: OverviewResult(items: manyItems(20), hasMore: true),
      );
      final vm = OverviewViewModel(getOverview);
      final billing = _TestBillingManager()..debugSetAccess(fullAccess: false);

      await tester.pumpWidget(_wrap(
        OverviewWidget(
          viewModel: vm,
          getCandleSeries: _FakeGetCandleSeries(),
          billingManager: billing,
        ),
      ));
      await tester.pumpAndSettle();

      expect(getOverview.pageCalls, [1]);

      // Scroll all the way to the bottom of the (small, capped) grid.
      await tester.fling(
          find.byType(GridView), const Offset(0, -3000), 3000);
      await tester.pumpAndSettle();

      // hasMore is true on the underlying result, but the free-tier cap
      // is showing — loadNext must not fire for data the cap won't
      // display anyway.
      expect(getOverview.pageCalls, [1]);
    });
  });

  group('lifecycle manager reparenting', () {
    testWidgets(
        're-registers with the new AppLifecycleManager when reparented under a different AppLifecycleScope',
        (WidgetTester tester) async {
      final vm = OverviewViewModel(_FakeGetOverview(
        result: const OverviewResult(items: [], hasMore: false),
      ));
      final managerA = AppLifecycleManager();
      final managerB = AppLifecycleManager();

      Widget buildUnder(AppLifecycleManager manager) {
        return MaterialApp(
          home: Scaffold(
            body: AppLifecycleScope(
              manager: manager,
              child: OverviewWidget(
                key: const ValueKey('overview'),
                viewModel: vm,
                getCandleSeries: _FakeGetCandleSeries(),
              ),
            ),
          ),
        );
      }

      await tester.pumpWidget(buildUnder(managerA));
      await tester.pumpAndSettle();

      expect(managerA.pausableCount, 1,
          reason: 'expected the widget to register with its initial manager');
      expect(managerB.pausableCount, 0);

      // Reparent the SAME widget (stable key, so its State persists) under
      // a different AppLifecycleScope — didChangeDependencies fires again
      // with a different manager instance.
      await tester.pumpWidget(buildUnder(managerB));
      await tester.pumpAndSettle();

      expect(managerA.pausableCount, 0,
          reason: 'expected the old manager\'s registration to be removed, not leaked');
      expect(managerB.pausableCount, 1,
          reason: 'expected the registration to move to the new manager, not be skipped');
    });
  });
}
