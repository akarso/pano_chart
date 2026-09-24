import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/market_state/participation_counts.dart';

void main() {
  group('ParticipationCounts', () {
    test('fromJson parses counts and shares', () {
      final p = ParticipationCounts.fromJson({
        'up': 62,
        'down': 8,
        'ranging': 30,
        'total': 100,
      });
      expect(p.up, 62);
      expect(p.down, 8);
      expect(p.ranging, 30);
      expect(p.total, 100);
      expect(p.upShare, 0.62);
      expect(p.hasData, isTrue);
    });

    test('fromJson null / wrong type / negatives yield safe empty', () {
      expect(ParticipationCounts.fromJson(null).hasData, isFalse);
      expect(ParticipationCounts.fromJson('nope').hasData, isFalse);
      expect(ParticipationCounts.fromJson([1, 2, 3]).hasData, isFalse);
      final neg = ParticipationCounts.fromJson({
        'up': -5,
        'down': 10,
        'ranging': 0,
        'total': 10,
      });
      expect(neg.up, 0);
      expect(neg.down, 10);
      expect(neg.total, 10);
    });

    test('hasData false when all parts 0', () {
      final p = ParticipationCounts.fromJson({
        'up': 0,
        'down': 0,
        'ranging': 0,
        'total': 100,
      });
      expect(p.hasData, isFalse);
    });

    test('renormalizes total when it disagrees with parts', () {
      final p = ParticipationCounts.fromJson({
        'up': 80,
        'down': 10,
        'ranging': 10,
        'total': 10,
      });
      expect(p.total, 100);
      expect(p.upShare, closeTo(0.8, 1e-9));
      expect(p.hasData, isTrue);
    });

    test('participationPercents sum to 100', () {
      final (u, r, d) = participationPercents(const ParticipationCounts(
        up: 1,
        down: 1,
        ranging: 1,
        total: 3,
      ));
      expect(u + r + d, 100);
    });
  });

  group('participationReading', () {
    test('broad with tape when lead matches trend bias', () {
      expect(
        participationReading(
          const ParticipationCounts(up: 62, down: 8, ranging: 30, total: 100),
          bias: 'up',
          regime: 'trend',
        ),
        'Broad move — most tokens trend with the tape',
      );
    });

    test('broad against the tape when lead opposes bias', () {
      expect(
        participationReading(
          const ParticipationCounts(up: 62, down: 8, ranging: 30, total: 100),
          bias: 'down',
          regime: 'trend',
        ),
        'Broad move against the tape',
      );
    });

    test('mixed against the tape', () {
      expect(
        participationReading(
          const ParticipationCounts(up: 40, down: 20, ranging: 40, total: 100),
          bias: 'down',
          regime: 'trend',
        ),
        'Mixed — tokens lean against the tape',
      );
    });

    test('narrow against the tape', () {
      expect(
        participationReading(
          const ParticipationCounts(up: 20, down: 15, ranging: 65, total: 100),
          bias: 'down',
          regime: 'trend',
        ),
        'Narrow — a few tokens lean against the tape',
      );
    });

    test('narrow when lead < 30%', () {
      expect(
        participationReading(
          const ParticipationCounts(up: 20, down: 15, ranging: 65, total: 100),
          bias: 'up',
          regime: 'trend',
        ),
        'Narrow — tape led by a few heavyweights',
      );
    });

    test('tied up/down under a downtape does not claim against the tape', () {
      expect(
        participationReading(
          const ParticipationCounts(up: 25, down: 25, ranging: 50, total: 100),
          bias: 'down',
          regime: 'trend',
        ),
        'Narrow — up and down are tied',
      );
    });

    test('reading thresholds follow rounded bar percents', () {
      // 29.6% raw would be Narrow on floats; largest-remainder can yield 30.
      final p = const ParticipationCounts(up: 37, down: 38, ranging: 50, total: 125);
      final (up, ranging, down) = participationPercents(p);
      expect(up + ranging + down, 100);
      // Whatever the rounded lead is, the reading must use those same ints.
      final reading = participationReading(p, bias: 'up', regime: 'trend');
      final lead = up >= down ? up : down;
      if (up >= 30 && down >= 30) {
        expect(reading, 'Split market');
      } else if (lead >= 50) {
        expect(reading, contains('Broad'));
      } else if (lead >= 30) {
        expect(reading, contains('Mixed'));
      } else {
        expect(reading, contains('Narrow'));
      }
    });

    test('mixed when lead is 30–50%', () {
      expect(
        participationReading(
          const ParticipationCounts(up: 40, down: 20, ranging: 40, total: 100),
          bias: 'up',
          regime: 'trend',
        ),
        'Mixed — tape led by part of the market',
      );
    });

    test('split when up and down both ≥ 30%', () {
      expect(
        participationReading(
          const ParticipationCounts(up: 35, down: 35, ranging: 30, total: 100),
          bias: 'up',
          regime: 'trend',
        ),
        'Split market',
      );
    });

    test('non-trend does not claim agreement with the tape', () {
      expect(
        participationReading(
          const ParticipationCounts(up: 62, down: 8, ranging: 30, total: 100),
          bias: 'neutral',
          regime: 'sideways',
        ),
        'Broad up — most tokens are trending up',
      );
    });
  });
}
