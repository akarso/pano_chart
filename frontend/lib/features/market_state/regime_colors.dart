import 'package:flutter/material.dart';

/// Color for a dominant-regime string ("trend"/"sideways"/"compression"/
/// "expansion"/"indecisive"/"silent"). Shared across Market Pulse, the
/// overview grid's aligned badge, and the symbol detail MTF strip (PR-100)
/// so they can never disagree about what a regime looks like — mirrors
/// `MarketPulseScreen._regimeColor`, which delegates here.
Color regimeColor(String regime) {
  switch (regime) {
    case 'compression':
      return Colors.amber;
    case 'sideways':
      return Colors.blueGrey;
    case 'trend':
      return Colors.tealAccent;
    case 'expansion':
      return Colors.redAccent;
    case 'silent':
      return Colors.white;
    case 'indecisive':
      return const Color(0xFFB0C4DE);
    default:
      return Colors.grey;
  }
}

/// Bias-aware color for a trend reading ("up"/"down"/neutral).
Color trendBiasColor(String bias) {
  switch (bias) {
    case 'up':
      return Colors.tealAccent;
    case 'down':
      return Colors.redAccent;
    default:
      return Colors.amber;
  }
}

/// Bias-aware arrow icon for a trend reading ("up"/"down"/neutral).
IconData trendBiasIcon(String bias) {
  switch (bias) {
    case 'up':
      return Icons.trending_up;
    case 'down':
      return Icons.trending_down;
    default:
      return Icons.show_chart;
  }
}
