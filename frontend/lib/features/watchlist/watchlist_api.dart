/// How long one watchlist HTTP call may run before the client closes the
/// socket. The controller does not wrap a second timeout on top of this.
const watchlistRequestTimeout = Duration(seconds: 15);

/// A rejected watchlist request. [statusCode] and [message] stay attached
/// so callers can tell a full list (400) from a rate limit (429).
class WatchlistRequestException implements Exception {
  final int statusCode;
  final String message;

  const WatchlistRequestException({
    required this.statusCode,
    required this.message,
  });

  @override
  String toString() => 'WatchlistRequestException($statusCode): $message';
}

/// Port for syncing the caller's watchlisted symbols with the backend
/// (ROADMAP PR-101). All three methods return the resulting watchlist —
/// the source of truth is the server, so a caller never needs a separate
/// fetch after a mutation.
abstract class WatchlistApi {
  /// Fetches the caller's current watchlist.
  Future<List<String>> fetch();

  /// Replaces the caller's entire watchlist with [symbols] (PUT semantics
  /// — the server drops anything not in this list and preserves the
  /// original add-order for symbols that were already present).
  Future<List<String>> replace(List<String> symbols);

  /// Removes [symbols] from the caller's watchlist, if present.
  Future<List<String>> remove(List<String> symbols);
}
