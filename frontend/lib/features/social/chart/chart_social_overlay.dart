import 'package:flutter/material.dart';

import 'social_chart_overlay_painter.dart';

/// Tappable overlay showing social post markers on the detail chart.
///
/// When the user taps near a marker, a popup shows the post titles.
class ChartSocialOverlay extends StatefulWidget {
  final List<SocialMarker> markers;

  /// Height of the price area. Lines stop here.
  final double? priceAreaHeight;

  const ChartSocialOverlay({
    Key? key,
    required this.markers,
    this.priceAreaHeight,
  }) : super(key: key);

  @override
  State<ChartSocialOverlay> createState() => _ChartSocialOverlayState();
}

class _ChartSocialOverlayState extends State<ChartSocialOverlay> {
  SocialMarker? _selected;

  @override
  Widget build(BuildContext context) {
    return Stack(
      children: [
        Positioned.fill(
          child: CustomPaint(
            painter: SocialChartOverlayPainter(
              markers: widget.markers,
              priceAreaHeight: widget.priceAreaHeight,
            ),
          ),
        ),
        Positioned.fill(
          child: GestureDetector(
            behavior: HitTestBehavior.translucent,
            onTapDown: _onTap,
          ),
        ),
        if (_selected != null) _buildPopup(context, _selected!),
      ],
    );
  }

  void _onTap(TapDownDetails details) {
    final tapX = details.localPosition.dx;
    const hitRadius = 20.0;

    SocialMarker? closest;
    double closestDist = double.infinity;
    for (final m in widget.markers) {
      final d = (m.x - tapX).abs();
      if (d < hitRadius && d < closestDist) {
        closest = m;
        closestDist = d;
      }
    }

    setState(() => _selected = closest);
  }

  Widget _buildPopup(BuildContext context, SocialMarker marker) {
    final left = (marker.x - 100).clamp(0.0, double.infinity);
    return Positioned(
      left: left,
      top: 16,
      child: GestureDetector(
        onTap: () => setState(() => _selected = null),
        child: Material(
          color: Colors.transparent,
          child: Container(
            width: 220,
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: Colors.grey[900]!.withAlpha(240),
              borderRadius: BorderRadius.circular(10),
              border: Border.all(color: kSocialBlue.withAlpha(80), width: 0.5),
            ),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  mainAxisAlignment: MainAxisAlignment.spaceBetween,
                  children: [
                    Text(
                      '${marker.posts.length} post${marker.posts.length > 1 ? 's' : ''}',
                      style: const TextStyle(
                        color: Colors.white70,
                        fontSize: 11,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                    InkWell(
                      onTap: () => setState(() => _selected = null),
                      child: const Icon(Icons.close,
                          size: 14, color: Colors.white54),
                    ),
                  ],
                ),
                const SizedBox(height: 6),
                for (final p in marker.posts.take(5)) ...[
                  _SocialPopupRow(post: p),
                  if (p != marker.posts.last) const SizedBox(height: 4),
                ],
                if (marker.posts.length > 5)
                  Padding(
                    padding: const EdgeInsets.only(top: 4),
                    child: Text(
                      '+${marker.posts.length - 5} more',
                      style: const TextStyle(
                          color: Colors.white38, fontSize: 10),
                    ),
                  ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _SocialPopupRow extends StatelessWidget {
  final dynamic post;

  const _SocialPopupRow({required this.post});

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Container(
          width: 4,
          height: 4,
          margin: const EdgeInsets.only(top: 5, right: 6),
          decoration: const BoxDecoration(
            color: kSocialBlue,
            shape: BoxShape.circle,
          ),
        ),
        Expanded(
          child: Text(
            post.title as String,
            maxLines: 2,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(color: Colors.white70, fontSize: 11),
          ),
        ),
      ],
    );
  }
}
