import '../../../domain/event.dart';

/// Response DTO wrapping a list of events from the backend.
class EventsResponseDto {
  final List<Event> events;

  const EventsResponseDto({required this.events});

  factory EventsResponseDto.fromJson(Map<String, dynamic> json) {
    final list = (json['events'] as List<dynamic>?) ?? [];
    return EventsResponseDto(
      events: list
          .map((e) => Event.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }
}
