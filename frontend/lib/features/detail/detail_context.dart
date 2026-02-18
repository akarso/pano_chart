/// Contextual ranking data passed from the overview to the detail screen.
///
/// Carries the rank position and individual score components so the detail
/// screen can display a score breakdown without re-fetching.
class DetailContext {
  final int rank;
  final double totalScore;
  final double trendScore;
  final double sidewaysScore;
  final double gainScore;
  final double volume;

  const DetailContext({
    required this.rank,
    required this.totalScore,
    required this.trendScore,
    required this.sidewaysScore,
    required this.gainScore,
    required this.volume,
  });
}
