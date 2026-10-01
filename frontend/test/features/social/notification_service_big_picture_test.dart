import 'dart:typed_data';

import 'package:flutter_local_notifications/flutter_local_notifications.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/social/notification_service.dart';

void main() {
  test(
    'buildDetails uses big-picture style when sparkline PNG is provided',
    () {
      final png = Uint8List.fromList([
        0x89,
        0x50,
        0x4E,
        0x47,
        0x0D,
        0x0A,
        0x1A,
        0x0A,
        0x00,
        0x00,
        0x00,
        0x00,
      ]);

      final details = NotificationService.buildDetails(
        title: 'Breakout starting',
        body: 'BTCUSDT (1h)',
        sparklinePng: png,
      );

      expect(details.android, isNotNull);
      expect(
        details.android!.styleInformation,
        isA<BigPictureStyleInformation>(),
      );
    },
  );

  test('buildDetails without sparkline keeps default style', () {
    final details = NotificationService.buildDetails(
      title: 'Hello',
      body: 'World',
    );

    expect(details.android, isNotNull);
    expect(details.android!.styleInformation, isNull);
  });

  testWidgets(
    'resolveDetails falls back to plain style when PNG render throws',
    (tester) async {
      final svc = NotificationService(
        renderSparkline: (_) async => throw StateError('paint failed'),
      );

      final details = await svc.resolveDetails(
        title: 'Breakout starting',
        body: 'BTCUSDT (1h)',
        sparkline: List<double>.generate(30, (i) => 100.0 + i),
      );

      expect(details.android, isNotNull);
      expect(details.android!.styleInformation, isNull);
    },
  );

  testWidgets(
    'show still invokes plugin with plain details after render failure',
    (tester) async {
      final plugin = _RecordingPlugin();
      final svc = NotificationService.withPlugin(
        plugin,
        renderSparkline: (_) async => throw StateError('paint failed'),
      );
      svc.markInitializedForTest();

      await svc.show(
        title: 'Breakout starting',
        body: 'BTCUSDT (1h)',
        sparkline: const [100.0, 101.0, 102.0],
      );

      expect(plugin.showCount, 1);
      expect(plugin.lastDetails, isNotNull);
      expect(plugin.lastDetails!.android!.styleInformation, isNull);
      expect(plugin.lastTitle, 'Breakout starting');
      expect(plugin.lastBody, 'BTCUSDT (1h)');
    },
  );
}

/// Minimal stand-in: only [show] is exercised after [markInitializedForTest].
class _RecordingPlugin extends Fake implements FlutterLocalNotificationsPlugin {
  int showCount = 0;
  NotificationDetails? lastDetails;
  String? lastTitle;
  String? lastBody;

  @override
  Future<void> show(
    int id,
    String? title,
    String? body,
    NotificationDetails? details, {
    String? payload,
  }) async {
    showCount++;
    lastTitle = title;
    lastBody = body;
    lastDetails = details;
  }
}
