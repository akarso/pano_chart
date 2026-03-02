import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import '../core/config/config.dart';
import '../core/di/di.dart';
import '../core/di/composition_root.dart';
import '../features/overview/overview_widget.dart';
import '../infrastructure/preferences_service.dart';

/// Exposed bootstrap function so tests can instantiate the app without side effects.
Widget bootstrapApp({required AppConfig config, PreferencesService? prefs}) {
  final root = CompositionRoot(apiBaseUrl: config.apiBaseUrl);
  final overviewViewModel = root.createOverviewViewModel();
  final getCandleSeries = root.createGetCandleSeries();
  final eventsViewModel = root.createEventsViewModel();
  final component = AppComponent(
    config,
    home: OverviewWidget(
      viewModel: overviewViewModel,
      getCandleSeries: getCandleSeries,
      eventsViewModel: eventsViewModel,
      prefs: prefs,
    ),
  );
  return component.createApp();
}

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  SystemChrome.setEnabledSystemUIMode(SystemUiMode.edgeToEdge);
  SystemChrome.setSystemUIOverlayStyle(const SystemUiOverlayStyle(
    statusBarColor: Color(0x00000000),
    systemNavigationBarColor: Color(0x00000000),
    systemNavigationBarDividerColor: Color(0x00000000),
  ));

  final prefs = await PreferencesService.create();

  const config = AppConfig(
      apiBaseUrl: 'http://srv1024540.hstgr.cloud:8080', flavor: 'dev');
  runApp(bootstrapApp(config: config, prefs: prefs));
}
