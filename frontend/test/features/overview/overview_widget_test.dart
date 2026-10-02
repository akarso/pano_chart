import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/core/app_lifecycle_manager.dart';
import 'package:pano_chart_frontend/features/billing/api/subscription_api.dart';
import 'package:pano_chart_frontend/features/billing/billing_manager.dart';
import 'package:pano_chart_frontend/features/billing/upgrade_screen.dart';
import 'package:pano_chart_frontend/features/candles/application/get_candle_series.dart';
import 'package:pano_chart_frontend/features/candles/application/get_candle_series_input.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/features/market_state/composite_index_data.dart';
import 'package:pano_chart_frontend/features/market_state/http_composite_index_api.dart';
import 'package:pano_chart_frontend/features/market_state/http_market_state_api.dart';
import 'package:pano_chart_frontend/features/market_state/market_pulse_screen.dart';
import 'package:pano_chart_frontend/features/market_state/market_state_data.dart';
import 'package:pano_chart_frontend/features/bubble_map/bubble_map_screen.dart';
import 'package:pano_chart_frontend/features/bubble_map/bubble_map_view_model.dart';
import 'package:pano_chart_frontend/features/detail/detail_screen.dart';
import 'package:pano_chart_frontend/features/detail/setup_detail_loader.dart';
import 'package:pano_chart_frontend/features/overview/overview_widget.dart';
import 'package:pano_chart_frontend/features/overview/overview_view_model.dart';
import 'package:pano_chart_frontend/features/overview/get_overview.dart';
import 'package:pano_chart_frontend/features/overview/overview_state.dart';
import 'package:pano_chart_frontend/features/replay/replay_asof.dart';
import 'package:pano_chart_frontend/features/replay/replay_controller.dart';
import 'package:pano_chart_frontend/features/scorecards/http_scorecard_api.dart';
import 'package:pano_chart_frontend/features/scorecards/scorecard_data.dart';
import 'package:pano_chart_frontend/features/watchlist/watchlist_api.dart';
import 'package:pano_chart_frontend/features/watchlist/watchlist_controller.dart';
import 'package:pano_chart_frontend/infrastructure/preferences_service.dart';
import 'package:shared_preferences/shared_preferences.dart';

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
  final List<bool> mtfCalls = [];
  final List<List<String>> symbolCalls = [];
  final List<int?> asOfCalls = [];

  _FakeGetOverview({this.delay = Duration.zero, required this.result});

  @override
  Future<OverviewResult> call({
    required String timeframe,
    required int page,
    required String sort,
    String? snapshot,
    String sidewaysAlgo = 'v1',
    List<String> symbols = const [],
    bool mtf = false,
    int? asOf,
  }) async {
    pageCalls.add(page);
    mtfCalls.add(mtf);
    symbolCalls.add(List.of(symbols));
    asOfCalls.add(asOf);
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

/// In-memory WatchlistApi fake — starts with [symbols] and records every
/// replace/remove call so tests can assert on what the widget synced.
///
/// Set [fetchError], [replaceError], or [removeError] to fail that call.
/// Set [holdMutations] to leave replace/remove unfinished until [release].
/// Set [holdFetch] to leave [fetch] unfinished until [releaseFetch].
class _FakeWatchlistApi implements WatchlistApi {
  List<String> symbols;
  final List<List<String>> replaceCalls = [];
  final List<List<String>> removeCalls = [];
  Object? fetchError;
  Object? replaceError;
  Object? removeError;
  bool holdMutations = false;
  bool holdFetch = false;
  int fetches = 0;
  final List<Completer<void>> _mutationHolds = [];
  Completer<void>? _fetchHold;

  _FakeWatchlistApi([List<String> initial = const []])
    : symbols = List.of(initial);

  void release() {
    final pending = List.of(_mutationHolds);
    _mutationHolds.clear();
    for (final hold in pending) {
      if (!hold.isCompleted) hold.complete();
    }
  }

  void releaseFetch() {
    final hold = _fetchHold;
    _fetchHold = null;
    if (hold != null && !hold.isCompleted) hold.complete();
  }

  Future<void> _maybeHold() {
    if (!holdMutations) return Future<void>.value();
    final hold = Completer<void>();
    _mutationHolds.add(hold);
    return hold.future;
  }

  @override
  Future<List<String>> fetch() async {
    fetches++;
    if (holdFetch) {
      _fetchHold = Completer<void>();
      await _fetchHold!.future;
    }
    if (fetchError != null) throw fetchError!;
    return symbols;
  }

  @override
  Future<List<String>> replace(List<String> newSymbols) async {
    replaceCalls.add(List.of(newSymbols));
    await _maybeHold();
    if (replaceError != null) throw replaceError!;
    symbols = List.of(newSymbols);
    return symbols;
  }

  @override
  Future<List<String>> remove(List<String> toRemove) async {
    removeCalls.add(List.of(toRemove));
    await _maybeHold();
    if (removeError != null) throw removeError!;
    symbols = symbols.where((s) => !toRemove.contains(s)).toList();
    return symbols;
  }
}

Widget _wrap(Widget w) => MaterialApp(home: Scaffold(body: w));

class _FakeScorecardApi implements ScorecardApi {
  final List<ScorecardSummaryItem> items;
  final List<String> sinceCalls = [];

  _FakeScorecardApi({this.items = const []});

  @override
  Future<ScorecardSummary> summary({
    required String timeframe,
    String since = '30d',
  }) async {
    sinceCalls.add(since);
    return ScorecardSummary(
      timeframe: timeframe,
      since: since,
      sinceRaw: since,
      items: items,
    );
  }

  @override
  Future<ScorecardDetail> get({
    required String kind,
    required String label,
    required String timeframe,
    String since = '30d',
  }) {
    throw UnimplementedError();
  }
}

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

  testWidgets('OverviewScreen_handlesEmptySparkline', (
    WidgetTester tester,
  ) async {
    final items = [const OverviewItem(symbol: 'BTCUSDT')];

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
    testWidgets(
      'shows disclaimer when sort-relevant scores are below threshold',
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

        final vm = OverviewViewModel(
          _FakeGetOverview(
            result: OverviewResult(items: items, hasMore: false),
          ),
        );
        vm.changeSortSilent('sideways');

        await tester.pumpWidget(
          _wrap(
            OverviewWidget(
              viewModel: vm,
              getCandleSeries: _FakeGetCandleSeries(),
            ),
          ),
        );
        await tester.pumpAndSettle();

        expect(find.textContaining('weakly represented'), findsOneWidget);
      },
    );

    testWidgets(
      'hides disclaimer when sort-relevant scores are above threshold',
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

        final vm = OverviewViewModel(
          _FakeGetOverview(
            result: OverviewResult(items: items, hasMore: false),
          ),
        );
        vm.changeSortSilent('sideways');

        await tester.pumpWidget(
          _wrap(
            OverviewWidget(
              viewModel: vm,
              getCandleSeries: _FakeGetCandleSeries(),
            ),
          ),
        );
        await tester.pumpAndSettle();

        expect(find.textContaining('weakly represented'), findsNothing);
      },
    );

    testWidgets('checks trend score when sorting by trend', (
      WidgetTester tester,
    ) async {
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

      final vm = OverviewViewModel(
        _FakeGetOverview(result: OverviewResult(items: items, hasMore: false)),
      );
      vm.changeSortSilent('trend');

      await tester.pumpWidget(
        _wrap(
          OverviewWidget(
            viewModel: vm,
            getCandleSeries: _FakeGetCandleSeries(),
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.textContaining('weakly represented'), findsOneWidget);
    });

    testWidgets('suppressed for volume/gain/losers sorts', (
      WidgetTester tester,
    ) async {
      final items = List.generate(
        5,
        (i) => OverviewItem(
          symbol: 'SYM${i}USDT',
          totalScore: 0.10, // very weak
          sparkline: const [100.0, 101.0],
        ),
      );

      for (final sort in ['volume', 'gain', 'losers']) {
        final vm = OverviewViewModel(
          _FakeGetOverview(
            result: OverviewResult(items: items, hasMore: false),
          ),
        );
        vm.changeSortSilent(sort);

        await tester.pumpWidget(
          _wrap(
            OverviewWidget(
              viewModel: vm,
              getCandleSeries: _FakeGetCandleSeries(),
            ),
          ),
        );
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

    testWidgets('free-tier user with >15 tokens sees the upgrade banner', (
      WidgetTester tester,
    ) async {
      final vm = OverviewViewModel(
        _FakeGetOverview(
          result: OverviewResult(items: manyItems(20), hasMore: false),
        ),
      );
      final billing = _TestBillingManager()..debugSetAccess(fullAccess: false);

      await tester.pumpWidget(
        _wrap(
          OverviewWidget(
            viewModel: vm,
            getCandleSeries: _FakeGetCandleSeries(),
            billingManager: billing,
          ),
        ),
      );
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

    testWidgets('pro user with >15 tokens does not see the upgrade banner', (
      WidgetTester tester,
    ) async {
      final vm = OverviewViewModel(
        _FakeGetOverview(
          result: OverviewResult(items: manyItems(20), hasMore: false),
        ),
      );
      final billing = _TestBillingManager()..debugSetAccess(fullAccess: true);

      await tester.pumpWidget(
        _wrap(
          OverviewWidget(
            viewModel: vm,
            getCandleSeries: _FakeGetCandleSeries(),
            billingManager: billing,
          ),
        ),
      );
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

    testWidgets('free-tier user with <=15 tokens does not see the banner', (
      WidgetTester tester,
    ) async {
      final vm = OverviewViewModel(
        _FakeGetOverview(
          result: OverviewResult(items: manyItems(10), hasMore: false),
        ),
      );
      final billing = _TestBillingManager()..debugSetAccess(fullAccess: false);

      await tester.pumpWidget(
        _wrap(
          OverviewWidget(
            viewModel: vm,
            getCandleSeries: _FakeGetCandleSeries(),
            billingManager: billing,
          ),
        ),
      );
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
        final billing = _TestBillingManager()
          ..debugSetAccess(fullAccess: false);

        await tester.pumpWidget(
          _wrap(
            OverviewWidget(
              viewModel: vm,
              getCandleSeries: _FakeGetCandleSeries(),
              billingManager: billing,
            ),
          ),
        );
        await tester.pumpAndSettle();

        expect(getOverview.pageCalls, [1]);

        // Scroll all the way to the bottom of the (small, capped) grid.
        await tester.fling(find.byType(GridView), const Offset(0, -3000), 3000);
        await tester.pumpAndSettle();

        // hasMore is true on the underlying result, but the free-tier cap
        // is showing — loadNext must not fire for data the cap won't
        // display anyway.
        expect(getOverview.pageCalls, [1]);
      },
    );
  });

  group('lifecycle manager reparenting', () {
    testWidgets(
      're-registers with the new AppLifecycleManager when reparented under a different AppLifecycleScope',
      (WidgetTester tester) async {
        final vm = OverviewViewModel(
          _FakeGetOverview(
            result: const OverviewResult(items: [], hasMore: false),
          ),
        );
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

        expect(
          managerA.pausableCount,
          1,
          reason: 'expected the widget to register with its initial manager',
        );
        expect(managerB.pausableCount, 0);

        // Reparent the SAME widget (stable key, so its State persists) under
        // a different AppLifecycleScope — didChangeDependencies fires again
        // with a different manager instance.
        await tester.pumpWidget(buildUnder(managerB));
        await tester.pumpAndSettle();

        expect(
          managerA.pausableCount,
          0,
          reason:
              'expected the old manager\'s registration to be removed, not leaked',
        );
        expect(
          managerB.pausableCount,
          1,
          reason:
              'expected the registration to move to the new manager, not be skipped',
        );
      },
    );
  });

  group('_requireAccess() fail-closed gating', () {
    testWidgets(
      'tapping a gated menu row with no billingManager does not unlock the screen',
      (WidgetTester tester) async {
        // Regression test for PR-078 CR follow-up (blocker): _requireAccess()
        // used to fail OPEN on a null billingManager
        // (`billing == null || billing.hasFullAccess` → true), so a gated
        // screen like Market Pulse would be reachable with no purchase
        // prompt at all whenever billing is unavailable — exactly the
        // fail-open leak this PR's Capabilities.fromBilling fix was
        // otherwise closing.
        final vm = OverviewViewModel(
          _FakeGetOverview(
            result: const OverviewResult(items: [], hasMore: false),
          ),
        );

        await tester.pumpWidget(
          _wrap(
            OverviewWidget(
              viewModel: vm,
              getCandleSeries: _FakeGetCandleSeries(),
              billingManager: null,
              marketStateApi: _NeverCalledMarketStateApi(),
              compositeIndexApi: _NeverCalledCompositeIndexApi(),
            ),
          ),
        );
        await tester.pumpAndSettle();

        await tester.tap(find.byKey(const ValueKey('overview-menu-nav-icon')));
        await tester.pumpAndSettle();

        expect(
          find.text('Market Pulse'),
          findsOneWidget,
          reason: 'the gated menu row itself should still be offered',
        );

        await tester.tap(find.text('Market Pulse'));
        await tester.pumpAndSettle();

        // Neither the gated screen nor an upgrade prompt should have
        // opened — a null billingManager means there's nothing to gate
        // against or launch a purchase flow through, so the tap must be a
        // no-op, not an unlock. (The menu itself closes on tap regardless
        // of access — that's by design, so "Market Pulse" no longer being
        // found here just reflects the menu closing, not navigation.)
        expect(find.byType(MarketPulseScreen), findsNothing);
        expect(find.byType(UpgradeScreen), findsNothing);
        // Still on the overview screen, not pushed anywhere else.
        expect(
          find.byKey(const ValueKey('overview-menu-nav-icon')),
          findsOneWidget,
        );
      },
    );
  });

  group('aligned badge (PR-100)', () {
    testWidgets(
      'shows a top border + tooltip for a pro user at alignment >= 0.75',
      (WidgetTester tester) async {
        final vm = OverviewViewModel(
          _FakeGetOverview(
            result: const OverviewResult(
              items: [
                OverviewItem(
                  symbol: 'ALIGNEDUSDT',
                  totalScore: 1.0,
                  sparkline: [100.0, 101.0],
                  alignment: 0.75,
                  alignedState: 'trend',
                ),
              ],
              hasMore: false,
            ),
          ),
        );
        final billing = _TestBillingManager()..debugSetAccess(fullAccess: true);

        await tester.pumpWidget(
          _wrap(
            OverviewWidget(
              viewModel: vm,
              getCandleSeries: _FakeGetCandleSeries(),
              billingManager: billing,
            ),
          ),
        );
        await tester.pumpAndSettle();

        expect(
          find.byKey(const Key('aligned-badge-ALIGNEDUSDT')),
          findsOneWidget,
        );
        expect(find.byTooltip('Aligned: trend (75%)'), findsOneWidget);
      },
    );

    testWidgets('hides the border below the 0.75 alignment floor', (
      WidgetTester tester,
    ) async {
      final vm = OverviewViewModel(
        _FakeGetOverview(
          result: const OverviewResult(
            items: [
              OverviewItem(
                symbol: 'WEAKUSDT',
                totalScore: 1.0,
                sparkline: [100.0, 101.0],
                alignment: 0.5,
                alignedState: 'trend',
              ),
            ],
            hasMore: false,
          ),
        ),
      );
      final billing = _TestBillingManager()..debugSetAccess(fullAccess: true);

      await tester.pumpWidget(
        _wrap(
          OverviewWidget(
            viewModel: vm,
            getCandleSeries: _FakeGetCandleSeries(),
            billingManager: billing,
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.byKey(const Key('aligned-badge-WEAKUSDT')), findsNothing);
    });

    testWidgets(
      'hides the border for a free-tier user even at alignment >= 0.75',
      (WidgetTester tester) async {
        final vm = OverviewViewModel(
          _FakeGetOverview(
            result: const OverviewResult(
              items: [
                OverviewItem(
                  symbol: 'ALIGNEDUSDT',
                  totalScore: 1.0,
                  sparkline: [100.0, 101.0],
                  alignment: 1.0,
                  alignedState: 'trend',
                ),
              ],
              hasMore: false,
            ),
          ),
        );
        final billing = _TestBillingManager()
          ..debugSetAccess(fullAccess: false);

        await tester.pumpWidget(
          _wrap(
            OverviewWidget(
              viewModel: vm,
              getCandleSeries: _FakeGetCandleSeries(),
              billingManager: billing,
            ),
          ),
        );
        await tester.pumpAndSettle();

        expect(
          find.byKey(const Key('aligned-badge-ALIGNEDUSDT')),
          findsNothing,
        );
      },
    );
  });

  group('mtf overlay request gating (PR-100 CR)', () {
    testWidgets('a free user does not request the ?mtf=1 overlay', (
      WidgetTester tester,
    ) async {
      final fakeGetOverview = _FakeGetOverview(
        result: const OverviewResult(items: [], hasMore: false),
      );
      final vm = OverviewViewModel(fakeGetOverview);
      final billing = _TestBillingManager()..debugSetAccess(fullAccess: false);

      await tester.pumpWidget(
        _wrap(
          OverviewWidget(
            viewModel: vm,
            getCandleSeries: _FakeGetCandleSeries(),
            billingManager: billing,
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(fakeGetOverview.mtfCalls, isNotEmpty);
      expect(fakeGetOverview.mtfCalls.every((v) => v == false), isTrue);
    });

    testWidgets('a pro user requests the ?mtf=1 overlay', (
      WidgetTester tester,
    ) async {
      final fakeGetOverview = _FakeGetOverview(
        result: const OverviewResult(items: [], hasMore: false),
      );
      final vm = OverviewViewModel(fakeGetOverview);
      final billing = _TestBillingManager()..debugSetAccess(fullAccess: true);

      await tester.pumpWidget(
        _wrap(
          OverviewWidget(
            viewModel: vm,
            getCandleSeries: _FakeGetCandleSeries(),
            billingManager: billing,
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(fakeGetOverview.mtfCalls, isNotEmpty);
      expect(fakeGetOverview.mtfCalls.every((v) => v == true), isTrue);
    });
  });

  group('watchlist (PR-101)', () {
    final items = [
      const OverviewItem(
        symbol: 'BTCUSDT',
        totalScore: 2.75,
        sparkline: [100.0, 105.0, 110.0],
      ),
      const OverviewItem(
        symbol: 'ETHUSDT',
        totalScore: -1.5,
        sparkline: [200.0, 195.0, 190.0],
      ),
    ];

    testWidgets(
      'tapping the tile star adds the symbol locally and syncs a PUT',
      (tester) async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        prefs.hasSeenAbout = true;
        final watchlistApi = _FakeWatchlistApi();
        final watchlist = WatchlistController(prefs: prefs, api: watchlistApi);
        final vm = OverviewViewModel(
          _FakeGetOverview(
            result: OverviewResult(items: items, hasMore: false),
          ),
        );

        await tester.pumpWidget(
          _wrap(
            OverviewWidget(
              viewModel: vm,
              getCandleSeries: _FakeGetCandleSeries(),
              prefs: prefs,
              watchlist: watchlist,
            ),
          ),
        );
        await tester.pumpAndSettle();

        // Scoped to the grid — the nav-bar "Watchlist" FilterChip has its own
        // star/star_border avatar icon that would otherwise also match.
        final tileStars = find.descendant(
          of: find.byType(GridView),
          matching: find.byIcon(Icons.star_border),
        );
        expect(tileStars, findsNWidgets(2));
        expect(
          find.descendant(
            of: find.byType(GridView),
            matching: find.byIcon(Icons.star),
          ),
          findsNothing,
        );

        await tester.tap(tileStars.first);
        await tester.pumpAndSettle();

        expect(
          find.descendant(
            of: find.byType(GridView),
            matching: find.byIcon(Icons.star),
          ),
          findsOneWidget,
        );
        expect(prefs.favourites, hasLength(1));
        expect(watchlistApi.replaceCalls, hasLength(1));
        expect(watchlistApi.replaceCalls.single, prefs.favourites.toList());
      },
    );

    testWidgets(
      'tapping a starred tile removes it locally and syncs a DELETE',
      (tester) async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        prefs.hasSeenAbout = true;
        prefs.addFavourite('BTCUSDT');
        final watchlistApi = _FakeWatchlistApi(['BTCUSDT']);
        final watchlist = WatchlistController(prefs: prefs, api: watchlistApi);
        final vm = OverviewViewModel(
          _FakeGetOverview(
            result: OverviewResult(items: items, hasMore: false),
          ),
        );

        await tester.pumpWidget(
          _wrap(
            OverviewWidget(
              viewModel: vm,
              getCandleSeries: _FakeGetCandleSeries(),
              prefs: prefs,
              watchlist: watchlist,
            ),
          ),
        );
        await tester.pumpAndSettle();

        final tileStarFilled = find.descendant(
          of: find.byType(GridView),
          matching: find.byIcon(Icons.star),
        );
        expect(tileStarFilled, findsOneWidget);

        await tester.tap(tileStarFilled);
        await tester.pumpAndSettle();

        expect(
          find.descendant(
            of: find.byType(GridView),
            matching: find.byIcon(Icons.star),
          ),
          findsNothing,
        );
        expect(
          find.descendant(
            of: find.byType(GridView),
            matching: find.byIcon(Icons.star_border),
          ),
          findsNWidgets(2),
        );
        expect(prefs.favourites, isEmpty);
        expect(watchlistApi.removeCalls, hasLength(1));
        expect(watchlistApi.removeCalls.single, ['BTCUSDT']);
      },
    );

    testWidgets(
      'the Watchlist filter chip filters the grid to starred symbols',
      (tester) async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        prefs.hasSeenAbout = true;
        prefs.addFavourite('BTCUSDT');
        final vm = OverviewViewModel(
          _FakeGetOverview(
            result: OverviewResult(items: items, hasMore: false),
          ),
        );

        await tester.pumpWidget(
          _wrap(
            OverviewWidget(
              viewModel: vm,
              getCandleSeries: _FakeGetCandleSeries(),
              prefs: prefs,
            ),
          ),
        );
        await tester.pumpAndSettle();

        expect(find.text('BTC'), findsOneWidget);
        expect(find.text('ETH'), findsOneWidget);

        await tester.tap(find.widgetWithText(FilterChip, 'Watchlist'));
        await tester.pumpAndSettle();

        expect(find.text('BTC'), findsOneWidget);
        expect(find.text('ETH'), findsNothing);
      },
    );

    testWidgets(
      'reconciles the local cache with the server watchlist at startup',
      (tester) async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        prefs.hasSeenAbout = true;
        prefs.addFavourite('ETHUSDT');
        final watchlistApi = _FakeWatchlistApi(['BTCUSDT']);
        final watchlist = WatchlistController(prefs: prefs, api: watchlistApi);
        final vm = OverviewViewModel(
          _FakeGetOverview(
            result: OverviewResult(items: items, hasMore: false),
          ),
        );

        await tester.pumpWidget(
          _wrap(
            OverviewWidget(
              viewModel: vm,
              getCandleSeries: _FakeGetCandleSeries(),
              prefs: prefs,
              watchlist: watchlist,
            ),
          ),
        );
        await tester.pumpAndSettle();

        // First fetch unions the local cache with the server list and
        // uploads that once, so ETH is not dropped.
        expect(prefs.favourites, {'ETHUSDT', 'BTCUSDT'});
        expect(watchlistApi.replaceCalls.single.toSet(), {
          'ETHUSDT',
          'BTCUSDT',
        });
      },
    );

    testWidgets('a failed add rolls the star back and surfaces an error', (
      tester,
    ) async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.hasSeenAbout = true;
      final watchlistApi = _FakeWatchlistApi()
        ..replaceError = Exception('offline');
      final watchlist = WatchlistController(prefs: prefs, api: watchlistApi);

      await tester.pumpWidget(
        _wrap(
          OverviewWidget(
            viewModel: OverviewViewModel(
              _FakeGetOverview(
                result: OverviewResult(items: items, hasMore: false),
              ),
            ),
            getCandleSeries: _FakeGetCandleSeries(),
            prefs: prefs,
            watchlist: watchlist,
          ),
        ),
      );
      await tester.pumpAndSettle();

      await tester.tap(
        find
            .descendant(
              of: find.byType(GridView),
              matching: find.byIcon(Icons.star_border),
            )
            .first,
      );
      await tester.pumpAndSettle();

      expect(
        find.descendant(
          of: find.byType(GridView),
          matching: find.byIcon(Icons.star),
        ),
        findsNothing,
      );
      expect(prefs.favourites, isEmpty);
      expect(find.text(watchlistSyncFailedMessage), findsOneWidget);
    });

    testWidgets('a failed remove rolls the star back', (tester) async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.hasSeenAbout = true;
      prefs.addFavourite('BTCUSDT');
      final watchlistApi = _FakeWatchlistApi(['BTCUSDT'])
        ..removeError = Exception('offline');
      final watchlist = WatchlistController(prefs: prefs, api: watchlistApi);

      await tester.pumpWidget(
        _wrap(
          OverviewWidget(
            viewModel: OverviewViewModel(
              _FakeGetOverview(
                result: OverviewResult(items: items, hasMore: false),
              ),
            ),
            getCandleSeries: _FakeGetCandleSeries(),
            prefs: prefs,
            watchlist: watchlist,
          ),
        ),
      );
      await tester.pumpAndSettle();

      await tester.tap(
        find.descendant(
          of: find.byType(GridView),
          matching: find.byIcon(Icons.star),
        ),
      );
      await tester.pumpAndSettle();

      expect(
        find.descendant(
          of: find.byType(GridView),
          matching: find.byIcon(Icons.star),
        ),
        findsOneWidget,
      );
      expect(prefs.favourites, {'BTCUSDT'});
      expect(find.text(watchlistSyncFailedMessage), findsOneWidget);
    });

    testWidgets(
      'two taps on the same star before either sync finishes keep the last tap',
      (tester) async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        prefs.hasSeenAbout = true;
        final watchlistApi = _FakeWatchlistApi()..holdMutations = true;
        final watchlist = WatchlistController(prefs: prefs, api: watchlistApi);

        await tester.pumpWidget(
          _wrap(
            OverviewWidget(
              viewModel: OverviewViewModel(
                _FakeGetOverview(
                  result: OverviewResult(items: items, hasMore: false),
                ),
              ),
              getCandleSeries: _FakeGetCandleSeries(),
              prefs: prefs,
              watchlist: watchlist,
            ),
          ),
        );
        await tester.pumpAndSettle();

        final outline = find.descendant(
          of: find.byType(GridView),
          matching: find.byIcon(Icons.star_border),
        );
        await tester.tap(outline.first);
        await tester.pump();
        expect(watchlistApi.replaceCalls, hasLength(1));

        await tester.tap(
          find.descendant(
            of: find.byType(GridView),
            matching: find.byIcon(Icons.star),
          ),
        );
        await tester.pump();

        watchlistApi.release();
        await tester.pump();
        watchlistApi.release();
        await tester.pumpAndSettle();

        expect(prefs.favourites, isEmpty);
        expect(watchlist.contains('BTCUSDT'), isFalse);
        expect(
          find.descendant(
            of: find.byType(GridView),
            matching: find.byIcon(Icons.star),
          ),
          findsNothing,
        );
      },
    );

    testWidgets('a failed startup fetch keeps the local cache', (tester) async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.hasSeenAbout = true;
      prefs.addFavourite('ETHUSDT');
      final watchlistApi = _FakeWatchlistApi()
        ..fetchError = Exception('offline');
      final watchlist = WatchlistController(prefs: prefs, api: watchlistApi);

      await tester.pumpWidget(
        _wrap(
          OverviewWidget(
            viewModel: OverviewViewModel(
              _FakeGetOverview(
                result: OverviewResult(items: items, hasMore: false),
              ),
            ),
            getCandleSeries: _FakeGetCandleSeries(),
            prefs: prefs,
            watchlist: watchlist,
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(prefs.favourites, {'ETHUSDT'});
      expect(
        find.descendant(
          of: find.byType(GridView),
          matching: find.byIcon(Icons.star),
        ),
        findsOneWidget,
      );
    });

    testWidgets(
      'an in-flight toggle is not reverted by startup reconciliation',
      (tester) async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        prefs.hasSeenAbout = true;
        final watchlistApi = _FakeWatchlistApi(['ETHUSDT'])..holdFetch = true;
        final watchlist = WatchlistController(prefs: prefs, api: watchlistApi);

        await tester.pumpWidget(
          _wrap(
            OverviewWidget(
              viewModel: OverviewViewModel(
                _FakeGetOverview(
                  result: OverviewResult(items: items, hasMore: false),
                ),
              ),
              getCandleSeries: _FakeGetCandleSeries(),
              prefs: prefs,
              watchlist: watchlist,
            ),
          ),
        );
        await tester.pumpAndSettle();

        await tester.tap(
          find
              .descendant(
                of: find.byType(GridView),
                matching: find.byIcon(Icons.star_border),
              )
              .first,
        );
        await tester.pump();

        watchlistApi.releaseFetch();
        await tester.pumpAndSettle();

        expect(watchlist.contains('BTCUSDT'), isTrue);
        expect(watchlist.contains('ETHUSDT'), isTrue);
        expect(prefs.favourites, containsAll(['BTCUSDT', 'ETHUSDT']));
        expect(watchlistApi.replaceCalls, isNotEmpty);
      },
    );

    testWidgets(
      'starring from the detail screen updates the grid without a pop result',
      (tester) async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        prefs.hasSeenAbout = true;
        final watchlistApi = _FakeWatchlistApi();
        final watchlist = WatchlistController(prefs: prefs, api: watchlistApi);

        await tester.pumpWidget(
          _wrap(
            OverviewWidget(
              viewModel: OverviewViewModel(
                _FakeGetOverview(
                  result: OverviewResult(items: items, hasMore: false),
                ),
              ),
              getCandleSeries: _OneCandleSeries(),
              prefs: prefs,
              watchlist: watchlist,
            ),
          ),
        );
        await tester.pumpAndSettle();

        await tester.tap(find.text('BTC'));
        await tester.pumpAndSettle();

        await tester.tap(
          find.descendant(
            of: find.byType(AppBar),
            matching: find.byIcon(Icons.star_border),
          ),
        );
        await tester.pumpAndSettle();

        expect(prefs.favourites, {'BTCUSDT'});
        expect(
          find.descendant(
            of: find.byType(GridView, skipOffstage: false),
            matching: find.byIcon(Icons.star, skipOffstage: false),
            skipOffstage: false,
          ),
          findsOneWidget,
        );
      },
    );

    testWidgets('an empty server list uploads the local stars once', (
      tester,
    ) async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.hasSeenAbout = true;
      prefs.addFavourite('ETHUSDT');
      final watchlistApi = _FakeWatchlistApi();
      final watchlist = WatchlistController(prefs: prefs, api: watchlistApi);

      await tester.pumpWidget(
        _wrap(
          OverviewWidget(
            viewModel: OverviewViewModel(
              _FakeGetOverview(
                result: OverviewResult(items: items, hasMore: false),
              ),
            ),
            getCandleSeries: _FakeGetCandleSeries(),
            prefs: prefs,
            watchlist: watchlist,
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(prefs.favourites, {'ETHUSDT'});
      expect(prefs.watchlistMigrated, isTrue);
      expect(watchlistApi.replaceCalls, hasLength(1));
      expect(watchlistApi.replaceCalls.single, contains('ETHUSDT'));
      expect(
        find.descendant(
          of: find.byType(GridView),
          matching: find.byIcon(Icons.star),
        ),
        findsOneWidget,
      );
    });

    testWidgets('the tile star is a 48px button', (tester) async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.hasSeenAbout = true;

      await tester.pumpWidget(
        _wrap(
          OverviewWidget(
            viewModel: OverviewViewModel(
              _FakeGetOverview(
                result: OverviewResult(items: items, hasMore: false),
              ),
            ),
            getCandleSeries: _FakeGetCandleSeries(),
            prefs: prefs,
          ),
        ),
      );
      await tester.pumpAndSettle();

      final stars = find.descendant(
        of: find.byType(GridView),
        matching: find.byType(IconButton),
      );
      expect(stars, findsNWidgets(2));
      final size = tester.getSize(stars.first);
      expect(size.width, greaterThanOrEqualTo(48));
      expect(size.height, greaterThanOrEqualTo(48));
      expect(find.byTooltip('Add to watchlist'), findsWidgets);
    });

    testWidgets('the tile star keeps the symbol name clear on a phone', (
      tester,
    ) async {
      addTearDown(() => tester.binding.setSurfaceSize(null));
      addTearDown(tester.platformDispatcher.clearTextScaleFactorTestValue);

      Future<void> at({
        required Size size,
        required int columns,
        String badgeComponent = '',
        double textScale = 1,
        ScorecardApi? scorecardApi,
        bool assertReliabilityClear = false,
      }) async {
        tester.platformDispatcher.clearTextScaleFactorTestValue();
        if (textScale != 1) {
          tester.platformDispatcher.textScaleFactorTestValue = textScale;
        }
        await tester.binding.setSurfaceSize(size);
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        prefs.hasSeenAbout = true;
        prefs.columns = columns;
        final billing = _TestBillingManager()..debugSetAccess(fullAccess: true);

        await tester.pumpWidget(
          _wrap(
            OverviewWidget(
              // Fresh state each case so initState reloads columns.
              key: ValueKey(
                'geom-$size-$columns-$badgeComponent-$textScale-'
                '${scorecardApi != null}',
              ),
              viewModel: OverviewViewModel(
                _FakeGetOverview(
                  result: OverviewResult(
                    items: [
                      OverviewItem(
                        symbol: 'BTCUSDT',
                        totalScore: 1,
                        sparkline: [100, 101],
                        rs: 0.032,
                        beta: 1.1,
                        badgeComponent: badgeComponent,
                      ),
                    ],
                    hasMore: false,
                    rsAvailable: true,
                  ),
                ),
              ),
              getCandleSeries: _FakeGetCandleSeries(),
              prefs: prefs,
              billingManager: billing,
              scorecardApi: scorecardApi,
            ),
          ),
        );
        await tester.pumpAndSettle();

        expect(tester.takeException(), isNull);
        final card = tester.getSize(find.byType(Card).first);
        final starFinder = find.descendant(
          of: find.byType(GridView),
          matching: find.byType(IconButton),
        );
        final star = tester.getSize(starFinder.first);
        expect(star.height, lessThanOrEqualTo(card.height));
        final symbolFinder = find.byKey(const Key('overview-name-BTCUSDT'));
        expect(symbolFinder, findsOneWidget);
        final symbol = tester.getRect(symbolFinder);
        final starRect = tester.getRect(starFinder.first);
        expect(starRect.overlaps(symbol), isFalse);

        final rsChip = find.descendant(
          of: find.byType(Card).first,
          matching: find.byKey(const ValueKey('relative-strength-chip')),
        );
        if (rsChip.evaluate().isNotEmpty) {
          final rsRect = tester.getRect(rsChip);
          expect(rsRect.overlaps(symbol), isFalse);
          expect(rsRect.overlaps(starRect), isFalse);
        }

        final pctFinder = find.byKey(const Key('overview-pct-BTCUSDT'));
        final shortStarBeside =
            size.width <= 320 &&
            textScale <= 1 &&
            !assertReliabilityClear &&
            ((columns == 2 && badgeComponent.isEmpty) ||
                (columns == 3 && badgeComponent.isNotEmpty));
        if (shortStarBeside || (textScale > 1 && size.width <= 320)) {
          if (pctFinder.evaluate().isNotEmpty) {
            expect(tester.getRect(pctFinder).overlaps(starRect), isFalse);
          }
          // Lower half of the star must still hit the button, not the
          // percent row that used to cover it.
          expect(
            find
                .byTooltip('Add to watchlist')
                .hitTestable(at: const Alignment(0, 0.85)),
            findsOneWidget,
          );
        }

        final minTapOnCard = card.height >= 40
            ? 32.0
            : (card.height - 8).clamp(24.0, 32.0);
        final tightBadge =
            columns == 3 &&
            badgeComponent.isNotEmpty &&
            size.width <= 320 &&
            textScale <= 1 &&
            !assertReliabilityClear;
        if (tightBadge) {
          // Beside-star room is under 24px once the badge is reserved.
          // The leftover band under the name is also under 32px, so the
          // control moves beside the name at the largest tap size the
          // card allows.
          expect(symbol.width, greaterThan(0));
          expect(star.width, greaterThanOrEqualTo(minTapOnCard - 0.5));
          expect(star.height, greaterThanOrEqualTo(minTapOnCard - 0.5));
        } else if (textScale > 1) {
          // Large text: star moves beside the name when the leftover
          // band is too short for a tap target; still no overlap.
          expect(star.width, greaterThanOrEqualTo(minTapOnCard - 0.5));
          expect(star.height, greaterThanOrEqualTo(minTapOnCard - 0.5));
        } else if (!assertReliabilityClear) {
          expect(star.width, greaterThanOrEqualTo(32));
          expect(star.height, greaterThanOrEqualTo(32));
        }
        if (badgeComponent.isNotEmpty) {
          final badge = tester.getRect(
            find.byKey(const Key('overview-badge-BTCUSDT')),
          );
          expect(badge.overlaps(symbol), isFalse);
        }
        if (assertReliabilityClear) {
          final chip = find.byKey(
            const ValueKey('reliability-chip-badge|trend_up'),
          );
          expect(chip, findsOneWidget);
          final chipRect = tester.getRect(chip);
          expect(chipRect.overlaps(symbol), isFalse);
          expect(symbol.width, greaterThan(0));
          // 3-column phone tiles must keep price/RS (≥14px). The badge
          // column scales above that strip so the pill can stay too.
          if (columns == 3) {
            expect(pctFinder, findsOneWidget);
            expect(rsChip, findsOneWidget);
          }
          if (pctFinder.evaluate().isNotEmpty) {
            final pctRect = tester.getRect(pctFinder);
            expect(chipRect.overlaps(pctRect), isFalse);
            expect(symbol.overlaps(pctRect), isFalse);
          }
          if (rsChip.evaluate().isNotEmpty) {
            expect(chipRect.overlaps(tester.getRect(rsChip)), isFalse);
          }
          expect(chip.hitTestable(at: Alignment.center), findsOneWidget);
        }
      }

      for (final size in const [Size(320, 700), Size(390, 800)]) {
        await at(size: size, columns: 2);
        await at(size: size, columns: 3);
        await at(size: size, columns: 3, badgeComponent: 'trend');
      }
      // Below-name path: one column is tall enough that the star sits
      // under the symbol without indenting.
      await at(size: const Size(390, 800), columns: 1);
      // Large text: leftover band under the name can be under 8px on a
      // short tile; the star must stay clear of the name and the
      // relative-strength chip must not cover it or the star.
      await at(size: const Size(320, 700), columns: 2, textScale: 2);
      await at(
        size: const Size(320, 700),
        columns: 3,
        badgeComponent: 'trend',
        textScale: 2,
      );
      const reliabilityItems = [
        ScorecardSummaryItem(
          kind: 'badge',
          label: 'trend_up',
          hitRate: 0.58,
          baseline: 0.5,
          n: 412,
        ),
      ];
      // Reliability pill vs name/meta on phone tiles.
      await at(
        size: const Size(320, 700),
        columns: 2,
        badgeComponent: 'trend',
        scorecardApi: _FakeScorecardApi(items: reliabilityItems),
        assertReliabilityClear: true,
      );
      await at(
        size: const Size(320, 700),
        columns: 3,
        badgeComponent: 'trend',
        scorecardApi: _FakeScorecardApi(items: reliabilityItems),
        assertReliabilityClear: true,
      );
      // Scaled badge column reserves a 14px meta strip; at text scale 1.3
      // the name band would otherwise push meta under that strip.
      await at(
        size: const Size(320, 700),
        columns: 3,
        badgeComponent: 'trend',
        textScale: 1.3,
        scorecardApi: _FakeScorecardApi(items: reliabilityItems),
        assertReliabilityClear: true,
      );
      // Same tile at text scale 2: name is clipped above the strip so it
      // cannot paint into the price row.
      await at(
        size: const Size(320, 700),
        columns: 3,
        badgeComponent: 'trend',
        textScale: 2,
        scorecardApi: _FakeScorecardApi(items: reliabilityItems),
        assertReliabilityClear: true,
      );
      // Unscaled column leaves ~15px; including the 2px gap that would
      // starve meta, so the column must scale and keep price/RS.
      await at(
        size: const Size(380, 700),
        columns: 3,
        badgeComponent: 'trend',
        scorecardApi: _FakeScorecardApi(items: reliabilityItems),
        assertReliabilityClear: true,
      );
      await at(
        size: const Size(390, 800),
        columns: 3,
        badgeComponent: 'trend',
        scorecardApi: _FakeScorecardApi(items: reliabilityItems),
        assertReliabilityClear: true,
      );
    });

    testWidgets('a notification shade does not fetch the watchlist again', (
      tester,
    ) async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.hasSeenAbout = true;
      final watchlistApi = _FakeWatchlistApi();
      final watchlist = WatchlistController(prefs: prefs, api: watchlistApi);
      final manager = AppLifecycleManager();

      await tester.pumpWidget(
        MaterialApp(
          home: AppLifecycleScope(
            manager: manager,
            child: Scaffold(
              body: OverviewWidget(
                viewModel: OverviewViewModel(
                  _FakeGetOverview(
                    result: OverviewResult(items: items, hasMore: false),
                  ),
                ),
                getCandleSeries: _FakeGetCandleSeries(),
                prefs: prefs,
                watchlist: watchlist,
              ),
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(watchlistApi.fetches, 1);

      manager.didChangeAppLifecycleState(AppLifecycleState.inactive);
      manager.didChangeAppLifecycleState(AppLifecycleState.resumed);
      await tester.pump();
      expect(watchlistApi.fetches, 1);

      manager.didChangeAppLifecycleState(AppLifecycleState.paused);
      manager.didChangeAppLifecycleState(AppLifecycleState.resumed);
      await tester.pump();
      expect(watchlistApi.fetches, 2);
    });

    testWidgets(
      'turning the Watchlist chip on during startup loads the server set',
      (tester) async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        prefs.hasSeenAbout = true;
        final overview = _FakeGetOverview(
          result: OverviewResult(items: items, hasMore: false),
        );
        final watchlistApi = _FakeWatchlistApi(['XYZUSDT'])..holdFetch = true;
        final watchlist = WatchlistController(prefs: prefs, api: watchlistApi);

        await tester.pumpWidget(
          _wrap(
            OverviewWidget(
              viewModel: OverviewViewModel(overview),
              getCandleSeries: _FakeGetCandleSeries(),
              prefs: prefs,
              watchlist: watchlist,
            ),
          ),
        );
        await tester.pumpAndSettle();

        await tester.tap(find.widgetWithText(FilterChip, 'Watchlist'));
        await tester.pump();

        watchlistApi.releaseFetch();
        await tester.pumpAndSettle();

        expect(
          overview.symbolCalls.any((symbols) => symbols.contains('XYZUSDT')),
          isTrue,
        );
      },
    );

    testWidgets(
      'bubble map and the notification loader hold the same controller',
      (tester) async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        prefs.hasSeenAbout = true;
        prefs.addFavourite('BTCUSDT');
        final watchlistApi = _FakeWatchlistApi(['BTCUSDT']);
        final watchlist = WatchlistController(prefs: prefs, api: watchlistApi);

        await tester.pumpWidget(
          _wrap(
            OverviewWidget(
              viewModel: OverviewViewModel(
                _FakeGetOverview(
                  result: OverviewResult(items: items, hasMore: false),
                ),
              ),
              getCandleSeries: _OneCandleSeries(),
              prefs: prefs,
              watchlist: watchlist,
              bubbleMapViewModel: BubbleMapViewModel(
                _FakeGetOverview(
                  result: const OverviewResult(items: [], hasMore: false),
                ),
              ),
            ),
          ),
        );
        await tester.pumpAndSettle();

        await tester.tap(find.byKey(const ValueKey('overview-menu-nav-icon')));
        await tester.pumpAndSettle();
        await tester.tap(find.text('Bubble Map'));
        await tester.pump();
        await tester.pump(const Duration(milliseconds: 400));

        final bubble = tester.widget<BubbleMapScreen>(
          find.byType(BubbleMapScreen),
        );
        expect(identical(bubble.watchlist, watchlist), isTrue);

        Navigator.of(tester.element(find.byType(BubbleMapScreen))).pop();
        await tester.pump();
        await tester.pump(const Duration(milliseconds: 400));

        Navigator.of(tester.element(find.byType(OverviewWidget))).push(
          MaterialPageRoute<void>(
            builder: (_) => SetupDetailLoader(
              symbol: 'BTCUSDT',
              getCandleSeries: _OneCandleSeries(),
              watchlist: bubble.watchlist,
            ),
          ),
        );
        await tester.pumpAndSettle();

        final detail = tester.widget<DetailScreen>(find.byType(DetailScreen));
        expect(identical(detail.watchlist, watchlist), isTrue);
        expect(
          find.descendant(
            of: find.byType(AppBar),
            matching: find.byIcon(Icons.star),
          ),
          findsOneWidget,
        );
      },
    );
  });

  group('PR-112b replay scrubber', () {
    final replayItems = [
      const OverviewItem(
        symbol: 'BTCUSDT',
        totalScore: 2.75,
        sparkline: [100.0, 105.0, 110.0],
        badgeComponent: 'trend',
      ),
    ];

    testWidgets('scrubber steps emit bar-aligned asOf and pause auto-refresh',
        (tester) async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.hasSeenAbout = true;
      final overview = _FakeGetOverview(
        result: OverviewResult(items: replayItems, hasMore: false),
      );
      final scorecards = _FakeScorecardApi();
      final replay = ReplayController(reloadDebounce: Duration.zero);
      final billing = _TestBillingManager()..debugSetAccess(fullAccess: true);
      final now = DateTime.utc(2025, 9, 16, 10, 17);

      await tester.pumpWidget(
        _wrap(
          OverviewWidget(
            viewModel: OverviewViewModel(overview),
            getCandleSeries: _FakeGetCandleSeries(),
            prefs: prefs,
            billingManager: billing,
            replayController: replay,
            scorecardApi: scorecards,
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(overview.asOfCalls, [null]);
      expect(find.byKey(const Key('replay-toggle')), findsOneWidget);

      replay.enter(timeframe: '1h', now: now);
      await tester.pumpAndSettle();

      expect(find.byKey(const Key('replay-banner')), findsOneWidget);
      expect(find.byKey(const Key('replay-banner-footnote')), findsOneWidget);
      expect(find.byKey(const Key('replay-scrubber')), findsOneWidget);
      expect(overview.asOfCalls.length, 2);
      expect(overview.asOfCalls.last, replay.asOfUnixFor('1h', now: now));
      expect(
        overview.asOfCalls.last,
        alignAsOfToBar(now, '1h').millisecondsSinceEpoch ~/ 1000,
      );
      expect(scorecards.sinceCalls.last, isNot('30d'));
      expect(
        DateTime.parse(scorecards.sinceCalls.last).isBefore(replay.asOf!),
        isTrue,
      );

      final afterEnter = overview.asOfCalls.length;
      await tester.pump(const Duration(seconds: 30));
      expect(overview.asOfCalls.length, afterEnter,
          reason: 'auto-refresh must stay paused in replay');

      await tester.tap(find.byKey(const Key('replay-step-bar-minus')));
      await tester.pumpAndSettle();
      expect(overview.asOfCalls.length, afterEnter + 1);
      final stepped = DateTime.fromMillisecondsSinceEpoch(
        overview.asOfCalls.last! * 1000,
        isUtc: true,
      );
      expect(stepped.minute, 0);
      expect(stepped.second, 0);
      expect(
        stepped,
        alignAsOfToBar(now, '1h').subtract(const Duration(hours: 1)),
      );

      replay.exit();
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('replay-banner')), findsNothing);
      expect(overview.asOfCalls.last, isNull);
    });

    testWidgets('TF change under replay reloads even when aligned unix matches',
        (tester) async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.hasSeenAbout = true;
      prefs.timeframe = '4h';
      final overview = _FakeGetOverview(
        result: OverviewResult(items: replayItems, hasMore: false),
      );
      final replay = ReplayController(reloadDebounce: Duration.zero);
      final billing = _TestBillingManager()..debugSetAccess(fullAccess: true);
      final now = DateTime.utc(2025, 9, 16, 10, 17);

      await tester.pumpWidget(
        _wrap(
          OverviewWidget(
            viewModel: OverviewViewModel(overview),
            getCandleSeries: _FakeGetCandleSeries(),
            prefs: prefs,
            billingManager: billing,
            replayController: replay,
          ),
        ),
      );
      await tester.pumpAndSettle();

      replay.enter(timeframe: '4h', now: now);
      await tester.pumpAndSettle();
      final afterEnter = overview.asOfCalls.length;
      final asOf4h = overview.asOfCalls.last;
      expect(asOf4h, isNotNull);
      // 1h alignment of the same instant yields the same unix (08:00).
      expect(
        replay.setAsOf('1h', replay.asOf!, now: now),
        isFalse,
      );

      await tester.tap(find.byKey(const ValueKey('overview-settings-nav-icon')));
      await tester.pumpAndSettle();
      // Open the timeframe dropdown (shows current '4h'), then pick '1h'.
      await tester.tap(find.text('4h'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('1h').last);
      await tester.pumpAndSettle();

      expect(overview.asOfCalls.length, greaterThan(afterEnter),
          reason: 'TF change must reload even when setAsOf does not notify');
      expect(overview.asOfCalls.last, asOf4h);
    });

    testWidgets('exit restarts auto-refresh even if reload fails',
        (tester) async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.hasSeenAbout = true;
      final overview = _FailAfterNOverview(
        succeedCount: 2,
        result: OverviewResult(items: replayItems, hasMore: false),
      );
      final replay = ReplayController(reloadDebounce: Duration.zero);
      final billing = _TestBillingManager()..debugSetAccess(fullAccess: true);
      final now = DateTime.utc(2025, 9, 16, 10, 17);

      await tester.pumpWidget(
        _wrap(
          OverviewWidget(
            viewModel: OverviewViewModel(overview),
            getCandleSeries: _FakeGetCandleSeries(),
            prefs: prefs,
            billingManager: billing,
            replayController: replay,
          ),
        ),
      );
      await tester.pumpAndSettle();

      replay.enter(timeframe: '1h', now: now);
      await tester.pumpAndSettle();
      expect(overview.calls, 2);

      replay.exit();
      await tester.pumpAndSettle();
      // Call 3 (exit reload) failed — timer must still tick.
      final callsBefore = overview.calls;
      await tester.pump(const Duration(seconds: 30));
      expect(overview.calls, greaterThan(callsBefore),
          reason: 'auto-refresh must restart after exit even if reload failed');
    });

    testWidgets('free tier hides the replay toggle', (tester) async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.hasSeenAbout = true;
      final billing = _TestBillingManager()..debugSetAccess(fullAccess: false);

      await tester.pumpWidget(
        _wrap(
          OverviewWidget(
            viewModel: OverviewViewModel(
              _FakeGetOverview(
                result: OverviewResult(items: replayItems, hasMore: false),
              ),
            ),
            getCandleSeries: _FakeGetCandleSeries(),
            prefs: prefs,
            billingManager: billing,
            replayController: ReplayController(reloadDebounce: Duration.zero),
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('replay-toggle')), findsNothing);
    });
  });
}

/// Succeeds [succeedCount] times, then throws — for exit/reload failure tests.
class _FailAfterNOverview extends GetOverview {
  _FailAfterNOverview({required this.succeedCount, required this.result});

  final int succeedCount;
  final OverviewResult result;
  int calls = 0;

  @override
  Future<OverviewResult> call({
    required String timeframe,
    required int page,
    required String sort,
    String? snapshot,
    String sidewaysAlgo = 'v1',
    List<String> symbols = const [],
    bool mtf = false,
    int? asOf,
  }) async {
    calls++;
    if (calls <= succeedCount) return result;
    throw Exception('network down');
  }
}

class _OneCandleSeries implements GetCandleSeries {
  @override
  Future<CandleSeriesResponse> execute(GetCandleSeriesInput input) async {
    return CandleSeriesResponse(
      symbol: input.symbol,
      timeframe: input.timeframe,
      candles: [
        CandleDto(
          timestamp: DateTime.utc(2025, 1, 1),
          open: 100,
          high: 105,
          low: 95,
          close: 102,
          volume: 1000,
        ),
      ],
    );
  }
}

class _NeverCalledMarketStateApi implements MarketStateApi {
  @override
  Future<MarketStateData> fetch({String timeframe = '4h'}) {
    fail(
      'MarketStateApi.fetch should never be called — access was not granted',
    );
  }
}

class _NeverCalledCompositeIndexApi implements CompositeIndexApi {
  @override
  Future<CompositeIndexData> fetch({String timeframe = '4h', int limit = 100, int? asOf}) {
    fail(
      'CompositeIndexApi.fetch should never be called — access was not granted',
    );
  }
}
