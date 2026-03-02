import 'events_response_dto.dart';

/// Port for fetching macroeconomic events from the backend.
abstract class EventsApi {
  /// Fetches events between [dateFrom] and [dateTo] (YYYY-MM-DD).
  /// Optionally filter by [impact] and [country].
  Future<EventsResponseDto> fetchEvents({
    required String dateFrom,
    required String dateTo,
    String? impact,
    String? country,
  });
}
