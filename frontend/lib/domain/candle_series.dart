// ...existing code...
import '../../features/candles/api/candle_response.dart';

/// Minimal CandleSeries for ranking use case (domain, not DTO).
class CandleSeries {
  final List<CandleDto> candles;
  const CandleSeries({required this.candles});
}
