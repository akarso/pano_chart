import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;

import '../auth/auth_headers.dart';
import 'watchlist_api.dart';

/// Exception thrown by [HttpWatchlistApi] on a non-200 response or a
/// body that is not a list of strings. [message] includes the status and
/// the response body.
class HttpWatchlistApiException extends WatchlistRequestException {
  const HttpWatchlistApiException({
    required super.statusCode,
    required super.message,
  });

  @override
  String toString() => 'HttpWatchlistApiException($statusCode): $message';
}

/// HTTP adapter implementing [WatchlistApi] against
/// `GET/PUT/DELETE /api/watchlist` (ROADMAP PR-101). Auth is hard-required
/// by the backend for this endpoint (no legacy/log-only fallback), so
/// every call goes through [sendAuthenticated].
class HttpWatchlistApi implements WatchlistApi {
  /// Shared client. When null, each call opens its own client and closes
  /// it on timeout so the socket is aborted. Closing the shared app
  /// client would cancel every other request.
  final http.Client? client;
  final String baseUrl;
  final String? Function()? getAuthSecret;
  final Future<void> Function()? onUnauthorized;
  final Duration requestTimeout;

  /// Used when [client] is null. Tests supply a client whose [http.Client.close]
  /// records that the timeout aborted the call.
  @visibleForTesting
  final http.Client Function()? openClient;

  HttpWatchlistApi({
    this.client,
    required this.baseUrl,
    this.getAuthSecret,
    this.onUnauthorized,
    this.requestTimeout = watchlistRequestTimeout,
    this.openClient,
  });

  Uri get _uri => Uri.parse(baseUrl).replace(path: '/api/watchlist');

  bool get _ownsClient => client == null;

  http.Client _transport() => client ?? (openClient ?? http.Client.new)();

  Future<http.Response> _send(
    Future<http.Response> Function(
      http.Client transport,
      Map<String, String> headers,
    )
    issue, [
    Map<String, String>? extraHeaders,
  ]) async {
    final transport = _transport();
    var closed = false;
    void closeOwned() {
      if (!_ownsClient || closed) return;
      closed = true;
      transport.close();
    }

    try {
      return await sendAuthenticated(
        (headers) => issue(transport, headers).timeout(
          requestTimeout,
          onTimeout: () {
            closeOwned();
            throw TimeoutException('watchlist');
          },
        ),
        getAuthSecret,
        onUnauthorized,
        extraHeaders,
      );
    } finally {
      closeOwned();
    }
  }

  @override
  Future<List<String>> fetch() async {
    final response = await _send(
      (transport, headers) => transport.get(_uri, headers: headers),
    );
    return _symbolsOrThrow(response, 'fetch');
  }

  @override
  Future<List<String>> replace(List<String> symbols) async {
    final body = jsonEncode({'symbols': symbols});
    final response = await _send(
      (transport, headers) => transport.put(_uri, body: body, headers: headers),
      {'Content-Type': 'application/json'},
    );
    return _symbolsOrThrow(response, 'replace');
  }

  @override
  Future<List<String>> remove(List<String> symbols) async {
    final body = jsonEncode({'symbols': symbols});
    final response = await _send(
      (transport, headers) =>
          transport.delete(_uri, body: body, headers: headers),
      {'Content-Type': 'application/json'},
    );
    return _symbolsOrThrow(response, 'remove');
  }

  List<String> _symbolsOrThrow(http.Response response, String op) {
    if (response.statusCode != 200) {
      throw HttpWatchlistApiException(
        statusCode: response.statusCode,
        message: 'Watchlist $op error: ${response.statusCode} ${response.body}',
      );
    }
    final decoded = jsonDecode(response.body);
    if (decoded is! Map<String, dynamic>) {
      throw HttpWatchlistApiException(
        statusCode: response.statusCode,
        message: 'Watchlist $op error: invalid JSON',
      );
    }
    final raw = decoded['symbols'];
    if (raw == null) return const [];
    if (raw is! List) {
      throw HttpWatchlistApiException(
        statusCode: response.statusCode,
        message: 'Watchlist $op error: symbols is not a list',
      );
    }
    final symbols = <String>[];
    for (final entry in raw) {
      if (entry is! String) {
        throw HttpWatchlistApiException(
          statusCode: response.statusCode,
          message: 'Watchlist $op error: symbol is not a string ($entry)',
        );
      }
      symbols.add(entry);
    }
    return symbols;
  }
}
