import 'package:flutter_test/flutter_test.dart';
import 'package:flutter/widgets.dart';
import 'package:pano_chart_frontend/bootstrap/main.dart' as bootstrap;
import 'package:pano_chart_frontend/core/config/config.dart';
import 'package:flutter/material.dart';

void main() {
  testWidgets('Root widget builds and shows placeholder', (WidgetTester tester) async {
    final widget = bootstrap.bootstrapApp(config: const AppConfig(apiBaseUrl: 'https://example', flavor: 'test'));
    await tester.pumpWidget(widget);
    await tester.pumpAndSettle();
    // Root placeholder contains the exact text defined in router
    expect(find.text('Root Placeholder'), findsOneWidget);
  });
}
