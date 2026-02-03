import '../config/config.dart';
import '../../app/app.dart';

/// Minimal DI container: composes the root App widget with configuration.
class AppComponent {
  final AppConfig config;

  AppComponent(this.config);

  /// Returns the root App widget wired with the provided config.
  App createApp() => App(config: config);
}
