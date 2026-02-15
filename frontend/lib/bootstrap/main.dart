import 'package:flutter/widgets.dart';
import '../core/config/config.dart';
import '../core/di/di.dart';
import '../core/di/composition_root.dart';
import '../features/overview/overview_widget.dart';

/// Exposed bootstrap function so tests can instantiate the app without side effects.
Widget bootstrapApp({required AppConfig config}) {
  final root = CompositionRoot(apiBaseUrl: config.apiBaseUrl);
  final overviewViewModel = root.createOverviewViewModel();
  final component = AppComponent(
    config,
    home: OverviewWidget(viewModel: overviewViewModel),
  );
  return component.createApp();
}

void main() {
  // In real apps these values could come from compile-time variables or CI.
  const config = AppConfig(apiBaseUrl: 'https://api.example', flavor: 'dev');
  runApp(bootstrapApp(config: config));
}
