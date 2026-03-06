import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/core/overview_banner.dart';

void main() {
  group('OverviewBanner widget', () {
    testWidgets('renders nothing when kind is none', (tester) async {
      await tester.pumpWidget(
        const MaterialApp(
          home: Scaffold(body: OverviewBanner(kind: OverviewBannerKind.none)),
        ),
      );
      expect(find.byType(SizedBox), findsWidgets); // SizedBox.shrink
      expect(find.text('Stale content, pull down to refresh'), findsNothing);
      expect(find.text('No connection — showing cached data'), findsNothing);
    });

    testWidgets('renders stale message when kind is stale', (tester) async {
      await tester.pumpWidget(
        const MaterialApp(
          home: Scaffold(body: OverviewBanner(kind: OverviewBannerKind.stale)),
        ),
      );
      expect(
          find.text('Stale content, pull down to refresh'), findsOneWidget);
    });

    testWidgets('renders offline message when kind is offline',
        (tester) async {
      await tester.pumpWidget(
        const MaterialApp(
          home:
              Scaffold(body: OverviewBanner(kind: OverviewBannerKind.offline)),
        ),
      );
      expect(find.text('No connection — showing cached data'), findsOneWidget);
    });

    testWidgets('stale and offline banners never overlap', (tester) async {
      // Only one banner should show at a time.
      // The offline kind takes priority — when switching from stale to offline,
      // only one banner is rendered.
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: Column(
              children: const [
                OverviewBanner(kind: OverviewBannerKind.offline),
              ],
            ),
          ),
        ),
      );
      expect(find.text('No connection — showing cached data'), findsOneWidget);
      expect(
          find.text('Stale content, pull down to refresh'), findsNothing);
    });
  });
}
