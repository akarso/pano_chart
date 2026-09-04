/// Shared values/helpers for the `dataQuality` field carried by
/// [MarketStateData] and [RegimeData] — see PR-074.
///
/// Kept as a plain string field on each model (matching how `state`/`regime`
/// are also raw strings from the API) rather than an enum, so this file
/// exists only to avoid duplicating the "unavailable" check in both models.

const String dataQualityUnavailable = 'unavailable';

/// Whether a `dataQuality` value indicates a full evaluation-source outage —
/// as opposed to a real (if quiet, if degraded) market read.
bool isDataQualityUnavailable(String dataQuality) =>
    dataQuality == dataQualityUnavailable;
