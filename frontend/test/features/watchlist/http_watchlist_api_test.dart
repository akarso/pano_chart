import 'dart:async';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pano_chart_frontend/features/watchlist/http_watchlist_api.dart';

class _HangingClient extends http.BaseClient {
  var closed = false;

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) {
    return Completer<http.StreamedResponse>().future;
  }

  @override
  void close() {
    closed = true;
    super.close();
  }
}

void main() {
  group('HttpWatchlistApi', () {
    test('fetch sends GET and parses symbols', () async {
      final client = MockClient((request) async {
        expect(request.method, 'GET');
        expect(request.url.path, '/api/watchlist');
        return http.Response(
          jsonEncode({
            'symbols': ['BTCUSDT', 'ETHUSDT'],
          }),
          200,
        );
      });

      final api = HttpWatchlistApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      final symbols = await api.fetch();
      expect(symbols, ['BTCUSDT', 'ETHUSDT']);
    });

    test(
      'fetch attaches Authorization header when a secret is available',
      () async {
        String? authHeader;
        final client = MockClient((request) async {
          authHeader = request.headers['Authorization'];
          return http.Response(jsonEncode({'symbols': []}), 200);
        });

        final api = HttpWatchlistApi(
          client: client,
          baseUrl: 'http://localhost:8080',
          getAuthSecret: () => 'my-secret',
        );

        await api.fetch();
        expect(authHeader, 'Bearer my-secret');
      },
    );

    test(
      'fetch throws a request exception when a symbol is not a string',
      () async {
        final client = MockClient(
          (request) async => http.Response(
            jsonEncode({
              'symbols': ['BTCUSDT', 1],
            }),
            200,
          ),
        );
        final api = HttpWatchlistApi(
          client: client,
          baseUrl: 'http://localhost:8080',
        );

        expect(
          () => api.fetch(),
          throwsA(
            isA<HttpWatchlistApiException>().having(
              (error) => error.message,
              'message',
              contains('not a string'),
            ),
          ),
        );
      },
    );

    test('fetch throws on non-200', () async {
      final client = MockClient((request) async => http.Response('error', 401));
      final api = HttpWatchlistApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      expect(() => api.fetch(), throwsA(isA<HttpWatchlistApiException>()));
    });

    test(
      'replace sends PUT with the symbols body and parses the response',
      () async {
        Map<String, dynamic>? sentBody;
        final client = MockClient((request) async {
          expect(request.method, 'PUT');
          expect(request.url.path, '/api/watchlist');
          expect(request.headers['Content-Type'], contains('application/json'));
          sentBody = jsonDecode(request.body) as Map<String, dynamic>;
          return http.Response(
            jsonEncode({
              'symbols': ['BTCUSDT', 'SOLUSDT'],
            }),
            200,
          );
        });

        final api = HttpWatchlistApi(
          client: client,
          baseUrl: 'http://localhost:8080',
        );

        final result = await api.replace(['BTCUSDT', 'SOLUSDT']);
        expect(sentBody!['symbols'], ['BTCUSDT', 'SOLUSDT']);
        expect(result, ['BTCUSDT', 'SOLUSDT']);
      },
    );

    test('replace throws on non-200', () async {
      final client = MockClient((request) async => http.Response('error', 400));
      final api = HttpWatchlistApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      expect(
        () => api.replace(['BTCUSDT']),
        throwsA(isA<HttpWatchlistApiException>()),
      );
    });

    test(
      'remove sends DELETE with the symbols body and parses the response',
      () async {
        Map<String, dynamic>? sentBody;
        final client = MockClient((request) async {
          expect(request.method, 'DELETE');
          expect(request.url.path, '/api/watchlist');
          sentBody = jsonDecode(request.body) as Map<String, dynamic>;
          return http.Response(
            jsonEncode({
              'symbols': ['BTCUSDT'],
            }),
            200,
          );
        });

        final api = HttpWatchlistApi(
          client: client,
          baseUrl: 'http://localhost:8080',
        );

        final result = await api.remove(['SOLUSDT']);
        expect(sentBody!['symbols'], ['SOLUSDT']);
        expect(result, ['BTCUSDT']);
      },
    );

    test('remove throws on non-200', () async {
      final client = MockClient((request) async => http.Response('error', 500));
      final api = HttpWatchlistApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      expect(
        () => api.remove(['BTCUSDT']),
        throwsA(isA<HttpWatchlistApiException>()),
      );
    });

    test('a timeout closes the client opened for that call', () async {
      final hanging = _HangingClient();
      final api = HttpWatchlistApi(
        baseUrl: 'http://localhost:8080',
        openClient: () => hanging,
        requestTimeout: const Duration(milliseconds: 20),
      );

      await expectLater(api.fetch(), throwsA(isA<TimeoutException>()));
      expect(hanging.closed, isTrue);
    });
  });
}
