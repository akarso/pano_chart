import 'http_scorecard_api.dart';
import 'scorecard_data.dart';

/// One in-flight summary per screen. A newer load drops older responses.
/// A timeframe change clears chips, including when that fetch fails. A failed
/// reload of the same timeframe keeps the summary already on screen.
class ScorecardCatalog {
  Map<String, ScorecardSummaryItem> items = const {};
  int _generation = 0;
  String? timeframe;

  Future<void> load({
    required ScorecardApi? api,
    required String timeframe,
    required void Function() notify,
  }) async {
    if (api == null) return;
    final generation = ++_generation;
    final switched = this.timeframe != timeframe;
    this.timeframe = timeframe;
    if (switched) {
      items = const {};
      notify();
    }
    try {
      final summary = await api.summary(timeframe: timeframe, since: '30d');
      if (generation != _generation) return;
      items = indexScorecards(summary.items);
      notify();
    } catch (_) {
      if (generation != _generation || !switched) return;
      items = const {};
      notify();
    }
  }
}
