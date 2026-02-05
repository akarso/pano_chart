/// Minimal OverviewViewModel stub for PR-011 test and widget wiring.
import '../../features/candles/api/candle_response.dart';
import '../../domain/symbol.dart';
import '../../domain/timeframe.dart';

class OverviewViewModel {
  final Timeframe timeframe;
  const OverviewViewModel({required this.timeframe});

  CandleSeriesResponse getCandleSeries(AppSymbol symbol, Timeframe timeframe) {
    // Return dummy data for now
    return CandleSeriesResponse(
      symbol: symbol.value,
      timeframe: timeframe.value,
      candles: [],
    );
  }
}
