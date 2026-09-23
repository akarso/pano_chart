import 'dart:async';
import 'dart:convert';

import 'package:fake_async/fake_async.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pano_chart_frontend/features/scorecards/http_scorecard_api.dart';

void main() {
  test('summary requests the timeframe and parses items', () async {
    final client = MockClient((request) async {
      expect(request.url.path, '/api/scorecards/summary');
      expect(request.url.queryParameters['timeframe'], '1h');
      expect(request.url.queryParameters['since'], '30d');
      return http.Response(
        jsonEncode({
          'timeframe': '1h',
          'since': '2026-08-22T00:00:00Z',
          'sinceRaw': '30d',
          'items': [
            {
              'kind': 'badge',
              'label': 'trend_up',
              'hitRate': 0.58,
              'baseline': null,
              'n': 412,
            },
          ],
        }),
        200,
      );
    });

    final api = HttpScorecardApi(
      client: client,
      baseUrl: 'http://localhost:8080',
    );
    final summary = await api.summary(timeframe: '1h');
    expect(summary.items.single.label, 'trend_up');
    expect(summary.items.single.baseline, isNull);
  });

  test('get requests kind and label', () async {
    final client = MockClient((request) async {
      expect(request.url.path, '/api/scorecards');
      expect(request.url.queryParameters['kind'], 'badge');
      expect(request.url.queryParameters['label'], 'trend_up');
      return http.Response(
        jsonEncode({
          'kind': 'badge',
          'label': 'trend_up',
          'timeframe': '1h',
          'since': '2026-08-22T00:00:00Z',
          'total': 1,
          'hits': 1,
          'hitRate': 1,
          'baseline': 0,
          'buckets': [],
        }),
        200,
      );
    });

    final api = HttpScorecardApi(
      client: client,
      baseUrl: 'http://localhost:8080',
    );
    final card = await api.get(
      kind: 'badge',
      label: 'trend_up',
      timeframe: '1h',
    );
    expect(card.baseline, 0);
    expect(card.buckets, isEmpty);
  });

  test('non-200 throws', () async {
    final client = MockClient((request) async => http.Response('nope', 502));
    final api = HttpScorecardApi(
      client: client,
      baseUrl: 'http://localhost:8080',
    );
    expect(
      () => api.summary(timeframe: '1h'),
      throwsA(isA<HttpScorecardApiException>()),
    );
  });

  test('a timed-out summary closes the client it opened', () {
    fakeAsync((async) {
      final spy = _HangingClient();
      final api = _OwnedApi(spy);
      Object? error;
      api
          .summary(timeframe: '4h')
          .then<void>(
            (_) {},
            onError: (Object e, StackTrace _) {
              error = e;
            },
          );
      async.elapse(const Duration(seconds: 15));
      async.flushMicrotasks();
      expect(spy.closed, isTrue);
      expect(error, isA<HttpScorecardApiException>());
    });
  });
}

class _OwnedApi extends HttpScorecardApi {
  _OwnedApi(this.spy) : super(baseUrl: 'http://localhost:8080');

  final http.Client spy;

  @override
  http.Client createOwnedClient() => spy;
}

class _HangingClient extends http.BaseClient {
  bool closed = false;
  final Completer<http.StreamedResponse> _pending = Completer();

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) {
    return _pending.future;
  }

  @override
  void close() {
    closed = true;
    if (!_pending.isCompleted) {
      _pending.completeError(StateError('closed'), StackTrace.empty);
    }
  }
}
