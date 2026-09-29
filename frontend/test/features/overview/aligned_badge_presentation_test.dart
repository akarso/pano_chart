import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/overview/aligned_badge_presentation.dart';

void main() {
  group('alignedTooltip', () {
    test('formats as a percentage for the full 4-frame case', () {
      expect(alignedTooltip('trend', 1.0), 'Aligned: trend (100%)');
      expect(alignedTooltip('trend', 0.75), 'Aligned: trend (75%)');
    });

    test('is still correct with fewer than 4 fresh frames', () {
      // 1.0 here could mean 1/1, 2/2, or 3/3 — a fixed "N/4" phrasing would
      // misreport all of these as "4/4"; the percentage has no such
      // ambiguity since it doesn't depend on how many frames contributed.
      expect(alignedTooltip('trend', 1.0), 'Aligned: trend (100%)');
      expect(alignedTooltip('sideways', 0.5), 'Aligned: sideways (50%)');
    });
  });
}
