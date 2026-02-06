import '../../domain/symbol.dart';
// ...existing code...

class RankedSymbol {
  final AppSymbol symbol;
  final Map<String, double> scores;
  final double totalScore;

  const RankedSymbol({
    required this.symbol,
    required this.scores,
    required this.totalScore,
  });
}
