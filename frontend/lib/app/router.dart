import 'package:flutter/widgets.dart';
import 'package:flutter/material.dart';

/// Centralized router definition with a single placeholder route.
class AppRouter {
  static Route<dynamic> generate(RouteSettings settings) {
    switch (settings.name) {
      case '/':
      default:
        return MaterialPageRoute(builder: (_) => const _RootPlaceholder());
    }
  }
}

class _RootPlaceholder extends StatelessWidget {
  const _RootPlaceholder({Key? key}) : super(key: key);

  @override
  Widget build(BuildContext context) {
    return const Scaffold(
      body: Center(child: Text('Root Placeholder')),
    );
  }
}
