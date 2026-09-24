import 'market_state_data.dart';
import 'participation_counts.dart';
import 'regime_data.dart';

/// Prefer healthy regime fields, else healthy state (Market Pulse headline).
String selectRegimeSource(RegimeData? regime, MarketStateData? state) {
  if (regime != null && !regime.isDataUnavailable) return regime.regimeSource;
  if (state != null && !state.isDataUnavailable) return state.regimeSource;
  return '';
}

String selectHeadlineBias(RegimeData? regime, MarketStateData? state) {
  if (regime != null && !regime.isDataUnavailable) return regime.bias;
  if (state != null && !state.isDataUnavailable) return state.bias;
  return '';
}

String selectHeadlineRegime(RegimeData? regime, MarketStateData? state) {
  if (regime != null && !regime.isDataUnavailable) return regime.regime;
  if (state != null && !state.isDataUnavailable) return state.state;
  return '';
}

ParticipationCounts? selectParticipationCounts(
  RegimeData? regime,
  MarketStateData? state,
) {
  if (regime != null &&
      !regime.isDataUnavailable &&
      regime.participation.hasData) {
    return regime.participation;
  }
  if (state != null &&
      !state.isDataUnavailable &&
      state.participation.hasData) {
    return state.participation;
  }
  return null;
}

/// Resolved participation card inputs (counts + reading + bar percents).
class ParticipationCardModel {
  final ParticipationCounts counts;
  final String reading;
  final int upPct;
  final int rangingPct;
  final int downPct;

  const ParticipationCardModel({
    required this.counts,
    required this.reading,
    required this.upPct,
    required this.rangingPct,
    required this.downPct,
  });

  /// Null when neither regime nor state has participation to show.
  static ParticipationCardModel? resolve({
    RegimeData? regime,
    MarketStateData? state,
  }) {
    final counts = selectParticipationCounts(regime, state);
    if (counts == null) return null;
    final (upPct, rangingPct, downPct) = participationPercents(counts);
    return ParticipationCardModel(
      counts: counts,
      reading: participationReading(
        counts,
        bias: selectHeadlineBias(regime, state),
        regime: selectHeadlineRegime(regime, state),
      ),
      upPct: upPct,
      rangingPct: rangingPct,
      downPct: downPct,
    );
  }
}
