import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/app/app.dart';
import 'package:pano_chart_frontend/core/config/config.dart';

void main() {
  testWidgets('App builds with AppConfig', (WidgetTester tester) async {
    const config =
        AppConfig(apiBaseUrl: 'https://api.example', flavor: 'test');
    await tester.pumpWidget(const App(config: config));
    expect(find.byType(App), findsOneWidget);
  });
}
