/// Data model for the Fear & Greed Index response.
class FearGreedData {
  final int value;
  final String classification;
  final DateTime timestampUtc;

  const FearGreedData({
    required this.value,
    required this.classification,
    required this.timestampUtc,
  });

  factory FearGreedData.fromJson(Map<String, dynamic> json) {
    return FearGreedData(
      value: json['value'] as int,
      classification: json['valueClassification'] as String,
      timestampUtc: DateTime.parse(json['timestampUtc'] as String),
    );
  }
}
