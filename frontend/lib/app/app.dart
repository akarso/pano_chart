import 'package:flutter/material.dart';
import '../core/config/config.dart';

/// Root App widget. Receives `AppConfig` via constructor injection.
/// Optionally accepts a [home] widget to render as the root screen.
class App extends StatelessWidget {
  final AppConfig config;
  final Widget? home;

  const App({Key? key, required this.config, this.home}) : super(key: key);

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Pano Chart',
      home: home != null ? Scaffold(body: home) : null,
      onGenerateRoute: home == null ? _placeholderRoute : null,
      theme: ThemeData.light(),
    );
  }

  static Route<dynamic> _placeholderRoute(RouteSettings settings) {
    return MaterialPageRoute(
      builder: (_) => const Scaffold(
        body: Center(child: Text('Root Placeholder')),
      ),
    );
  }
}
