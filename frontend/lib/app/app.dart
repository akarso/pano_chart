import 'package:flutter/material.dart';
import '../core/config/config.dart';
import 'router.dart';

/// Root App widget. Receives `AppConfig` via constructor injection.
class App extends StatelessWidget {
  final AppConfig config;

  const App({Key? key, required this.config}) : super(key: key);

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Pano Chart',
      onGenerateRoute: AppRouter.generate,
      // Provide a simple theme so tests can pump without errors.
      theme: ThemeData.light(),
    );
  }
}
