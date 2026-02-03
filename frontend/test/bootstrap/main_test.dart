import 'package:flutter_test/flutter_test.dart';
import 'package:flutter/widgets.dart';
import 'package:pano_chart_frontend/bootstrap/main.dart' as bootstrap;
import 'package:pano_chart_frontend/core/config/config.dart';

void main() {
  testWidgets('App starts without exceptions', (WidgetTester tester) async {
    final widget = bootstrap.bootstrapApp(
        config: const AppConfig(apiBaseUrl: 'https://example', flavor: 'test'));
    await tester.pumpWidget(widget);
    // pump and settle to ensure no exceptions during build
    await tester.pumpAndSettle();
  });
}
