/// Impact level for a macroeconomic event.
enum EventImpact {
  high,
  medium,
  low;

  /// Parses impact from backend JSON string (case-insensitive).
  static EventImpact fromString(String value) {
    switch (value.toLowerCase()) {
      case 'high':
        return EventImpact.high;
      case 'medium':
        return EventImpact.medium;
      case 'low':
        return EventImpact.low;
      default:
        return EventImpact.medium;
    }
  }
}

/// A macroeconomic calendar event.
///
/// Immutable value object. Timestamps are always stored in UTC.
class Event {
  final String id;
  final String country;
  final String title;
  final EventImpact impact;
  final DateTime timestamp; // UTC

  const Event({
    required this.id,
    required this.country,
    required this.title,
    required this.impact,
    required this.timestamp,
  });

  /// Creates from backend JSON.
  factory Event.fromJson(Map<String, dynamic> json) {
    return Event(
      id: json['id'] as String,
      country: json['country'] as String,
      title: json['title'] as String,
      impact: EventImpact.fromString(json['impact'] as String),
      timestamp: DateTime.parse(json['timestamp'] as String).toUtc(),
    );
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        'country': country,
        'title': title,
        'impact': impact.name,
        'timestamp': timestamp.toUtc().toIso8601String(),
      };

  @override
  bool operator ==(Object other) =>
      identical(this, other) || (other is Event && id == other.id);

  @override
  int get hashCode => id.hashCode;

  @override
  String toString() => 'Event($id, $title, $impact, $timestamp)';
}
