import '../../infrastructure/preferences_service.dart';
import '../detail/mtf_strip_presentation.dart';

/// Valid timeframe values matching backend domain/timeframe.go.
const kTimeframes = ['1m', '5m', '15m', '1h', '4h', '1d'];

/// Valid timeframes for `watchlistTimeframe`. Same four frames the MTF
/// strip renders ([kMtfStripTimeframes]): the regime stack never produces
/// `1m`/`5m`, and the backend rejects them on `watchlist_timeframe`.
const kWatchlistTimeframes = kMtfStripTimeframes;

/// Notification preferences per category — synced with [PreferencesService].
///
/// Market regime is split into three independent toggles (uptrend, downtrend,
/// sideways) each with a configurable minimum dominance threshold and
/// timeframe. The `market` notification type is considered enabled when
/// **any** of the three sub-toggles is on.
class NotificationSettings {
  bool social;
  bool macroHigh;
  bool macroModerate;
  bool uptrend;
  bool downtrend;
  bool sideways;
  bool setupOfDay;
  bool news;
  bool watchlistTransitions;

  double uptrendMinDominance;
  double downtrendMinDominance;
  double sidewaysMinDominance;
  double setupMinScore;

  String uptrendTimeframe;
  String downtrendTimeframe;
  String sidewaysTimeframe;
  String setupTimeframe;
  String watchlistTimeframe;

  NotificationSettings({
    required this.social,
    required this.macroHigh,
    required this.macroModerate,
    required this.uptrend,
    required this.downtrend,
    required this.sideways,
    required this.setupOfDay,
    required this.news,
    this.watchlistTransitions = true,
    this.uptrendMinDominance = 0.75,
    this.downtrendMinDominance = 0.75,
    this.sidewaysMinDominance = 0.75,
    this.setupMinScore = 0.75,
    this.uptrendTimeframe = '1h',
    this.downtrendTimeframe = '1h',
    this.sidewaysTimeframe = '1h',
    this.setupTimeframe = '1h',
    this.watchlistTimeframe = '1h',
  });

  factory NotificationSettings.defaults() => NotificationSettings(
    social: true,
    macroHigh: true,
    macroModerate: true,
    uptrend: true,
    downtrend: true,
    sideways: true,
    setupOfDay: true,
    news: true,
    watchlistTransitions: true,
  );

  /// Loads current values from persisted preferences.
  factory NotificationSettings.fromPrefs(PreferencesService prefs) =>
      NotificationSettings(
        social: prefs.notificationsEnabled,
        macroHigh: prefs.notifyMacroHigh,
        macroModerate: prefs.notifyMacroModerate,
        uptrend: prefs.notifyUptrend,
        downtrend: prefs.notifyDowntrend,
        sideways: prefs.notifySideways,
        setupOfDay: prefs.notifySetupOfDay,
        news: prefs.notifyNews,
        watchlistTransitions: prefs.notifyWatchlistTransitions,
        uptrendMinDominance: prefs.uptrendMinDominance,
        downtrendMinDominance: prefs.downtrendMinDominance,
        sidewaysMinDominance: prefs.sidewaysMinDominance,
        setupMinScore: prefs.setupMinScore,
        uptrendTimeframe: prefs.uptrendTimeframe,
        downtrendTimeframe: prefs.downtrendTimeframe,
        sidewaysTimeframe: prefs.sidewaysTimeframe,
        setupTimeframe: prefs.setupTimeframe,
        watchlistTimeframe: acceptedWatchlistTimeframe(
          prefs.watchlistTimeframe,
        ),
      );

  /// Persists all values back to [PreferencesService].
  void save(PreferencesService prefs) {
    prefs.notificationsEnabled = social;
    prefs.notifyMacroHigh = macroHigh;
    prefs.notifyMacroModerate = macroModerate;
    prefs.notifyUptrend = uptrend;
    prefs.notifyDowntrend = downtrend;
    prefs.notifySideways = sideways;
    prefs.notifySetupOfDay = setupOfDay;
    prefs.notifyNews = news;
    prefs.notifyWatchlistTransitions = watchlistTransitions;
    prefs.uptrendMinDominance = uptrendMinDominance;
    prefs.downtrendMinDominance = downtrendMinDominance;
    prefs.sidewaysMinDominance = sidewaysMinDominance;
    prefs.setupMinScore = setupMinScore;
    prefs.uptrendTimeframe = uptrendTimeframe;
    prefs.downtrendTimeframe = downtrendTimeframe;
    prefs.sidewaysTimeframe = sidewaysTimeframe;
    prefs.setupTimeframe = setupTimeframe;
    prefs.watchlistTimeframe = watchlistTimeframe;
  }

  /// Returns `true` if the given notification [type] is enabled.
  bool isEnabled(String type) {
    switch (type) {
      case 'twitter':
        return social;
      case 'macro':
        return macroHigh || macroModerate;
      case 'market':
        return uptrend || downtrend || sideways;
      case 'setup':
        return setupOfDay;
      case 'news':
        return news;
      case 'watchlist_transition':
        return watchlistTransitions;
      default:
        return true;
    }
  }

  /// Converts to JSON matching the backend notification config DTO.
  Map<String, dynamic> toJson(String userId) => {
    'user_id': userId,
    'social': social,
    'macro_high': macroHigh,
    'macro_moderate': macroModerate,
    'news': news,
    'uptrend': uptrend,
    'downtrend': downtrend,
    'sideways': sideways,
    'setup_of_day': setupOfDay,
    'watchlist_transitions': watchlistTransitions,
    'uptrend_min_dominance': uptrendMinDominance,
    'downtrend_min_dominance': downtrendMinDominance,
    'sideways_min_dominance': sidewaysMinDominance,
    'setup_min_score': setupMinScore,
    'uptrend_timeframe': uptrendTimeframe,
    'downtrend_timeframe': downtrendTimeframe,
    'sideways_timeframe': sidewaysTimeframe,
    'setup_timeframe': setupTimeframe,
    'watchlist_timeframe': watchlistTimeframe,
  };

  /// Applies server-side config values onto this instance.
  void applyFromJson(Map<String, dynamic> json) {
    social = json['social'] as bool? ?? social;
    macroHigh = json['macro_high'] as bool? ?? macroHigh;
    macroModerate = json['macro_moderate'] as bool? ?? macroModerate;
    news = json['news'] as bool? ?? news;
    uptrend = json['uptrend'] as bool? ?? uptrend;
    downtrend = json['downtrend'] as bool? ?? downtrend;
    sideways = json['sideways'] as bool? ?? sideways;
    setupOfDay = json['setup_of_day'] as bool? ?? setupOfDay;
    watchlistTransitions =
        json['watchlist_transitions'] as bool? ?? watchlistTransitions;
    uptrendMinDominance =
        (json['uptrend_min_dominance'] as num?)?.toDouble() ??
        uptrendMinDominance;
    downtrendMinDominance =
        (json['downtrend_min_dominance'] as num?)?.toDouble() ??
        downtrendMinDominance;
    sidewaysMinDominance =
        (json['sideways_min_dominance'] as num?)?.toDouble() ??
        sidewaysMinDominance;
    setupMinScore =
        (json['setup_min_score'] as num?)?.toDouble() ?? setupMinScore;
    uptrendTimeframe = json['uptrend_timeframe'] as String? ?? uptrendTimeframe;
    downtrendTimeframe =
        json['downtrend_timeframe'] as String? ?? downtrendTimeframe;
    sidewaysTimeframe =
        json['sideways_timeframe'] as String? ?? sidewaysTimeframe;
    setupTimeframe = json['setup_timeframe'] as String? ?? setupTimeframe;
    final rawWatchlistTf = json['watchlist_timeframe'];
    if (rawWatchlistTf is String &&
        kWatchlistTimeframes.contains(rawWatchlistTf)) {
      watchlistTimeframe = rawWatchlistTf;
    }
  }
}
