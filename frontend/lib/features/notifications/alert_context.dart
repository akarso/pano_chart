import 'dart:convert';

/// Parsed FCM / local-notification `context` blob (PR-102).
class AlertContext {
  /// Max sparkline points rendered / accepted from a payload.
  static const int maxSparklinePoints = 30;

  final String? tapeRegime;
  final String? tapeBias;
  final double? tapeConfidence;
  final double? symbolScore;
  final double? rs;
  final double? alignment;
  final List<double> sparkline;

  const AlertContext({
    this.tapeRegime,
    this.tapeBias,
    this.tapeConfidence,
    this.symbolScore,
    this.rs,
    this.alignment,
    this.sparkline = const [],
  });

  bool get hasSparkline => sparkline.length >= 2;

  /// Parses a JSON object map. Unknown / missing fields are ignored.
  factory AlertContext.fromJson(Map<String, dynamic>? json) {
    if (json == null) return const AlertContext();
    return AlertContext(
      tapeRegime: _asString(json['tapeRegime']),
      tapeBias: _asString(json['tapeBias']),
      tapeConfidence: _asDouble(json['tapeConfidence']),
      symbolScore: _asDouble(json['symbolScore']),
      rs: _asDouble(json['rs']),
      alignment: _asDouble(json['alignment']),
      sparkline: _asDoubleList(json['sparkline']),
    );
  }

  /// Parses the stringified `data["context"]` value. Bad JSON → empty.
  factory AlertContext.fromDataString(String? raw) {
    if (raw == null || raw.isEmpty) return const AlertContext();
    try {
      final decoded = json.decode(raw);
      if (decoded is Map<String, dynamic>) {
        return AlertContext.fromJson(decoded);
      }
      if (decoded is Map) {
        return AlertContext.fromJson(Map<String, dynamic>.from(decoded));
      }
    } catch (_) {
      // Malformed — treat as absent.
    }
    return const AlertContext();
  }

  /// Reads `context` from an FCM / local payload map.
  factory AlertContext.fromPayload(Map<String, dynamic>? data) {
    if (data == null) return const AlertContext();
    try {
      final raw = data['context'];
      if (raw is String) return AlertContext.fromDataString(raw);
      if (raw is Map<String, dynamic>) return AlertContext.fromJson(raw);
      if (raw is Map) {
        return AlertContext.fromJson(Map<String, dynamic>.from(raw));
      }
    } catch (_) {
      // Type skew on map-shaped context — treat as absent.
    }
    return const AlertContext();
  }

  static String? _asString(dynamic v) => v is String ? v : null;

  static double? _asDouble(dynamic v) {
    double? n;
    if (v is num) {
      n = v.toDouble();
    } else if (v is String) {
      n = double.tryParse(v);
    }
    if (n == null || n.isNaN || n.isInfinite) return null;
    return n;
  }

  static List<double> _asDoubleList(dynamic v) {
    if (v is! List) return const [];
    final out = <double>[];
    for (final item in v) {
      final n = _asDouble(item);
      if (n != null) out.add(n);
    }
    if (out.length > maxSparklinePoints) {
      return out.sublist(out.length - maxSparklinePoints);
    }
    return out;
  }
}
