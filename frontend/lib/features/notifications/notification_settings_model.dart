import '../../infrastructure/preferences_service.dart';

/// Notification preferences per category — synced with [PreferencesService].
///
/// Market regime is split into three independent toggles (uptrend, downtrend,
/// sideways) each with a configurable minimum dominance threshold. The
/// `market` notification type is considered enabled when **any** of the three
/// sub-toggles is on.
class NotificationSettings {
  bool social;
  bool macro;
  bool uptrend;
  bool downtrend;
  bool sideways;
  bool setupOfDay;
  bool news;

  double uptrendMinDominance;
  double downtrendMinDominance;
  double sidewaysMinDominance;
  double setupMinScore;

  NotificationSettings({
    required this.social,
    required this.macro,
    required this.uptrend,
    required this.downtrend,
    required this.sideways,
    required this.setupOfDay,
    required this.news,
    this.uptrendMinDominance = 0.75,
    this.downtrendMinDominance = 0.75,
    this.sidewaysMinDominance = 0.75,
    this.setupMinScore = 0.75,
  });

  factory NotificationSettings.defaults() => NotificationSettings(
        social: true,
        macro: true,
        uptrend: true,
        downtrend: true,
        sideways: true,
        setupOfDay: true,
        news: true,
      );

  /// Loads current values from persisted preferences.
  factory NotificationSettings.fromPrefs(PreferencesService prefs) =>
      NotificationSettings(
        social: prefs.notificationsEnabled,
        macro: prefs.notifyMacro,
        uptrend: prefs.notifyUptrend,
        downtrend: prefs.notifyDowntrend,
        sideways: prefs.notifySideways,
        setupOfDay: prefs.notifySetupOfDay,
        news: prefs.notifyNews,
        uptrendMinDominance: prefs.uptrendMinDominance,
        downtrendMinDominance: prefs.downtrendMinDominance,
        sidewaysMinDominance: prefs.sidewaysMinDominance,
        setupMinScore: prefs.setupMinScore,
      );

  /// Persists all values back to [PreferencesService].
  void save(PreferencesService prefs) {
    prefs.notificationsEnabled = social;
    prefs.notifyMacro = macro;
    prefs.notifyUptrend = uptrend;
    prefs.notifyDowntrend = downtrend;
    prefs.notifySideways = sideways;
    prefs.notifySetupOfDay = setupOfDay;
    prefs.notifyNews = news;
    prefs.uptrendMinDominance = uptrendMinDominance;
    prefs.downtrendMinDominance = downtrendMinDominance;
    prefs.sidewaysMinDominance = sidewaysMinDominance;
    prefs.setupMinScore = setupMinScore;
  }

  /// Returns `true` if the given notification [type] is enabled.
  bool isEnabled(String type) {
    switch (type) {
      case 'twitter':
        return social;
      case 'macro':
        return macro;
      case 'market':
        return uptrend || downtrend || sideways;
      case 'setup':
        return setupOfDay;
      case 'news':
        return news;
      default:
        return true;
    }
  }

  /// Converts to JSON matching the backend notification config DTO.
  Map<String, dynamic> toJson(String userId) => {
        'user_id': userId,
        'social': social,
        'macro': macro,
        'news': news,
        'uptrend': uptrend,
        'downtrend': downtrend,
        'sideways': sideways,
        'setup_of_day': setupOfDay,
        'uptrend_min_dominance': uptrendMinDominance,
        'downtrend_min_dominance': downtrendMinDominance,
        'sideways_min_dominance': sidewaysMinDominance,
        'setup_min_score': setupMinScore,
      };

  /// Applies server-side config values onto this instance.
  void applyFromJson(Map<String, dynamic> json) {
    social = json['social'] as bool? ?? social;
    macro = json['macro'] as bool? ?? macro;
    news = json['news'] as bool? ?? news;
    uptrend = json['uptrend'] as bool? ?? uptrend;
    downtrend = json['downtrend'] as bool? ?? downtrend;
    sideways = json['sideways'] as bool? ?? sideways;
    setupOfDay = json['setup_of_day'] as bool? ?? setupOfDay;
    uptrendMinDominance =
        (json['uptrend_min_dominance'] as num?)?.toDouble() ?? uptrendMinDominance;
    downtrendMinDominance =
        (json['downtrend_min_dominance'] as num?)?.toDouble() ?? downtrendMinDominance;
    sidewaysMinDominance =
        (json['sideways_min_dominance'] as num?)?.toDouble() ?? sidewaysMinDominance;
    setupMinScore =
        (json['setup_min_score'] as num?)?.toDouble() ?? setupMinScore;
  }
}
