import '../../../domain/event.dart';
import '../api/events_api.dart';

/// Input for the GetEvents use case.
class GetEventsInput {
  final String dateFrom;
  final String dateTo;
  final String? impact;
  final String? country;

  const GetEventsInput({
    required this.dateFrom,
    required this.dateTo,
    this.impact,
    this.country,
  });
}

/// Use case interface for fetching macroeconomic events.
abstract class GetEvents {
  Future<List<Event>> execute(GetEventsInput input);
}

/// Implementation that delegates to the [EventsApi] port.
class GetEventsImpl implements GetEvents {
  final EventsApi _api;

  GetEventsImpl(this._api);

  @override
  Future<List<Event>> execute(GetEventsInput input) async {
    final response = await _api.fetchEvents(
      dateFrom: input.dateFrom,
      dateTo: input.dateTo,
      impact: input.impact,
      country: input.country,
    );
    return response.events;
  }
}
