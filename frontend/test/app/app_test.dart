import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/app/app.dart';
import 'package:pano_chart_frontend/core/config/config.dart';

void main() {
  testWidgets('App without home shows placeholder',
      (WidgetTester tester) async {
    const config = AppConfig(apiBaseUrl: 'https://example', flavor: 'test');
    await tester.pumpWidget(const App(config: config));
    await tester.pumpAndSettle();
    expect(find.text('Root Placeholder'), findsOneWidget);
  });
}
