import '../config/config.dart';
import '../../app/app.dart';
import 'package:flutter/widgets.dart';

/// Minimal DI container: composes the root App widget with configuration.
class AppComponent {
  final AppConfig config;
  final Widget? home;

  AppComponent(this.config, {this.home});

  /// Returns the root App widget wired with the provided config.
  App createApp() => App(config: config, home: home);
}
