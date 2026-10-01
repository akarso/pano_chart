import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';

/// Renders [points] as a compact sparkline PNG for notification big-picture
/// style (PR-102). Caps input length to 30 points.
Future<Uint8List?> renderSparklinePng(
  List<double> points, {
  Size size = const Size(480, 160),
}) async {
  if (points.length < 2) return null;

  // Cap before PictureRecorder so a hostile/oversized payload cannot
  // force a large paint on the UI isolate.
  final capped = points.length > 30
      ? points.sublist(points.length - 30)
      : points;

  final recorder = ui.PictureRecorder();
  final canvas = Canvas(recorder);
  final rect = Offset.zero & size;

  canvas.drawRect(rect, Paint()..color = const Color(0xFF12141A));

  final first = capped.first;
  final last = capped.last;
  final Color lineColor;
  if (first == 0) {
    // % is undefined on a zero open — color by absolute move, not grey.
    lineColor = last == first
        ? const Color(0xFF9E9E9E)
        : (last > first
            ? const Color(0xFF4CAF50)
            : const Color(0xFFF44336));
  } else {
    final pct = ((last - first) / first) * 100;
    final rounded = pct.toStringAsFixed(1);
    final isZero = rounded == '0.0' || rounded == '-0.0';
    lineColor = isZero
        ? const Color(0xFF9E9E9E)
        : (last >= first
            ? const Color(0xFF4CAF50)
            : const Color(0xFFF44336));
  }

  final minVal = capped.reduce((a, b) => a < b ? a : b);
  final maxVal = capped.reduce((a, b) => a > b ? a : b);
  final range = (maxVal - minVal) == 0 ? 1.0 : (maxVal - minVal);
  final padY = size.height * 0.12;
  final usableH = size.height - 2 * padY;

  final path = Path();
  for (var i = 0; i < capped.length; i++) {
    final x = (i / (capped.length - 1)) * size.width;
    final y = padY + (1 - (capped[i] - minVal) / range) * usableH;
    if (i == 0) {
      path.moveTo(x, y);
    } else {
      path.lineTo(x, y);
    }
  }

  canvas.drawPath(
    path,
    Paint()
      ..color = lineColor
      ..strokeWidth = 3
      ..style = PaintingStyle.stroke
      ..strokeCap = StrokeCap.round
      ..strokeJoin = StrokeJoin.round,
  );

  final picture = recorder.endRecording();
  final image = await picture.toImage(size.width.toInt(), size.height.toInt());
  final byteData = await image.toByteData(format: ui.ImageByteFormat.png);
  image.dispose();
  return byteData?.buffer.asUint8List();
}
