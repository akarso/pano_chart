import 'package:flutter/widgets.dart';
import '../core/config/config.dart';
import '../core/di/di.dart';

/// Exposed bootstrap function so tests can instantiate the app without side effects.
Widget bootstrapApp({required AppConfig config}) {
  final component = AppComponent(config);
  return component.createApp();
}

void main() {
  // In real apps these values could come from compile-time variables or CI.
  const config = AppConfig(apiBaseUrl: 'https://api.example', flavor: 'dev');
  runApp(bootstrapApp(config: config));
}
