/// Input object for GetCandleSeries use case.
class GetCandleSeriesInput {
  final String symbol;
  final String timeframe;
  final DateTime from;
  final DateTime to;

  const GetCandleSeriesInput(
      {required this.symbol,
      required this.timeframe,
      required this.from,
      required this.to})
      : assert(symbol != null),
        assert(timeframe != null),
        assert(from != null),
        assert(to != null);
}
