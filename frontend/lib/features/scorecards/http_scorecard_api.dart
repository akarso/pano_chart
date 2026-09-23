import 'dart:async';
import 'dart:convert';

import 'package:http/http.dart' as http;

import 'scorecard_data.dart';

/// Scorecard reads. Routes are public (no device auth).
abstract class ScorecardApi {
  Future<ScorecardSummary> summary({
    required String timeframe,
    String since = '30d',
  });

  Future<ScorecardDetail> get({
    required String kind,
    required String label,
    required String timeframe,
    String since = '30d',
  });
}

class HttpScorecardApi implements ScorecardApi {
  /// When null, each call opens a client and closes it when the call
  /// finishes, including on timeout, so the socket is aborted.
  final http.Client? client;
  final String baseUrl;

  HttpScorecardApi({this.client, required this.baseUrl});

  @override
  Future<ScorecardSummary> summary({
    required String timeframe,
    String since = '30d',
  }) async {
    final uri = _uri('/api/scorecards/summary', {
      if (timeframe.isNotEmpty) 'timeframe': timeframe,
      'since': since,
    });
    final json = await _get(uri);
    return ScorecardSummary.fromJson(json);
  }

  @override
  Future<ScorecardDetail> get({
    required String kind,
    required String label,
    required String timeframe,
    String since = '30d',
  }) async {
    final uri = _uri('/api/scorecards', {
      'kind': kind,
      'label': label,
      if (timeframe.isNotEmpty) 'timeframe': timeframe,
      'since': since,
    });
    final json = await _get(uri);
    return ScorecardDetail.fromJson(json);
  }

  Uri _uri(String path, Map<String, String> query) {
    return Uri.parse('$baseUrl$path').replace(queryParameters: query);
  }

  Future<Map<String, dynamic>> _get(Uri uri) async {
    final response = await _send(uri);
    if (response.statusCode != 200) {
      throw HttpScorecardApiException(
        'Scorecard API error: ${response.statusCode}',
      );
    }
    final decoded = jsonDecode(response.body);
    if (decoded is! Map<String, dynamic>) {
      throw HttpScorecardApiException('Scorecard API error: invalid JSON');
    }
    return decoded;
  }

  /// Client opened when [client] was not injected. Closed when the call
  /// finishes, including on timeout. Tests override this to observe [close].
  http.Client createOwnedClient() => http.Client();

  Future<http.Response> _send(Uri uri) async {
    final injected = client;
    if (injected != null) {
      return injected.get(uri).timeout(const Duration(seconds: 15));
    }
    final owned = createOwnedClient();
    final future = owned.get(uri);
    try {
      return await future.timeout(const Duration(seconds: 15));
    } on TimeoutException {
      throw HttpScorecardApiException('Scorecard API error: timeout');
    } finally {
      owned.close();
      unawaited(
        future.then<void>((_) {}, onError: (Object _, StackTrace __) {}),
      );
    }
  }
}

class HttpScorecardApiException implements Exception {
  final String message;
  HttpScorecardApiException(this.message);

  @override
  String toString() => message;
}
