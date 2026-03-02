import '../../domain/event.dart';

/// Which impact levels to show on the chart overlay.
enum EventFilterLevel {
  /// High only
  highOnly,

  /// High + Medium (default)
  highAndMedium,

  /// All (High + Medium + Low)
  all;

  /// Returns true if [impact] passes this filter.
  bool allows(EventImpact impact) {
    switch (this) {
      case EventFilterLevel.highOnly:
        return impact == EventImpact.high;
      case EventFilterLevel.highAndMedium:
        return impact == EventImpact.high || impact == EventImpact.medium;
      case EventFilterLevel.all:
        return true;
    }
  }

  /// Human-readable label.
  String get label {
    switch (this) {
      case EventFilterLevel.highOnly:
        return 'High Only';
      case EventFilterLevel.highAndMedium:
        return 'High + Medium';
      case EventFilterLevel.all:
        return 'All';
    }
  }

  /// Parse from persisted string.
  static EventFilterLevel fromString(String value) {
    switch (value) {
      case 'highOnly':
        return EventFilterLevel.highOnly;
      case 'all':
        return EventFilterLevel.all;
      case 'highAndMedium':
      default:
        return EventFilterLevel.highAndMedium;
    }
  }
}
