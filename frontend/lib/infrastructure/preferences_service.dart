import 'dart:math';

import 'package:shared_preferences/shared_preferences.dart';

import '../features/social/api/social_account_settings.dart';

/// Persists user settings and favourites across app restarts.
///
/// Uses [SharedPreferences] as the backing store.
class PreferencesService {
    static const _keyUserId = 'device.userId';

    /// Returns the underlying [SharedPreferences] instance so other
    /// services (e.g. [TrialManager]) can share the same storage.
    SharedPreferences get sharedPreferences => _prefs;

    /// Stable device identifier — generated once and persisted forever.
    String get userId {
      var id = _prefs.getString(_keyUserId);
      if (id == null) {
        id = _generateUuid();
        _prefs.setString(_keyUserId, id);
      }
      return id;
    }

    /// Generates a v4 UUID using a cryptographically secure RNG.
    static String _generateUuid() {
      final rng = Random.secure();
      final bytes = List<int>.generate(16, (_) => rng.nextInt(256));
      bytes[6] = (bytes[6] & 0x0f) | 0x40; // version 4
      bytes[8] = (bytes[8] & 0x3f) | 0x80; // variant 1
      final hex =
          bytes.map((b) => b.toRadixString(16).padLeft(2, '0')).join();
      return '${hex.substring(0, 8)}-${hex.substring(8, 12)}-'
          '${hex.substring(12, 16)}-${hex.substring(16, 20)}-'
          '${hex.substring(20)}';
    }
    // ---- offline rankings cache ----
    static String _cacheKeyForTimeframe(String tf) => 'cache.rankings.$tf';
    static String _cacheTimestampKeyForTimeframe(String tf) => 'cache.rankings.$tf.ts';

    /// Stores the last successful rankings response for a timeframe as JSON.
    Future<void> setRankingsCache(String timeframe, String json) async {
      await _prefs.setString(_cacheKeyForTimeframe(timeframe), json);
      await _prefs.setString(_cacheTimestampKeyForTimeframe(timeframe), DateTime.now().toUtc().toIso8601String());
    }

    /// Gets the cached rankings JSON for a timeframe, or null if not present.
    String? getRankingsCache(String timeframe) => _prefs.getString(_cacheKeyForTimeframe(timeframe));

    /// Gets the cache timestamp for a timeframe, or null if not present.
    String? getRankingsCacheTimestamp(String timeframe) => _prefs.getString(_cacheTimestampKeyForTimeframe(timeframe));
  static const _keyColumns = 'settings.columns';
  static const _keyTimeframe = 'settings.timeframe';
  static const _keySort = 'settings.sort';
  static const _keySidewaysAlgo = 'settings.sidewaysAlgo';
  static const _keySortDirection = 'settings.sortDirection';
  static const _keyNormalize = 'settings.normalizeSparklines';
  static const _keyHiRes = 'settings.hiResSparklines';
  static const _keyExcludeStablecoins = 'settings.excludeStablecoins';
  static const _keyFavourites = 'favourites';
  static const _keyShowEvents = 'settings.showEvents';
  static const _keyEventFilter = 'settings.eventFilter';
  static const _keyPreferredExchange = 'settings.preferredExchange';
  static const _keySelectedCountries = 'settings.selectedCountries';
  static const _keyMacroInfluence = 'settings.macroInfluenceFilter';
  static const _keyCustomExchangeName = 'settings.customExchangeName';
  static const _keyCustomExchangeUrl = 'settings.customExchangeUrl';
  static const _keyHasSeenAbout = 'app.hasSeenAbout';

  final SharedPreferences _prefs;

  PreferencesService(this._prefs);

  /// Creates an instance asynchronously (call once at startup).
  static Future<PreferencesService> create() async {
    final prefs = await SharedPreferences.getInstance();
    return PreferencesService(prefs);
  }

  // ---- settings ----

  int get columns => _prefs.getInt(_keyColumns) ?? 2;
  set columns(int v) => _prefs.setInt(_keyColumns, v);

  String get timeframe => _prefs.getString(_keyTimeframe) ?? '1h';
  set timeframe(String v) => _prefs.setString(_keyTimeframe, v);

  String get sort => _prefs.getString(_keySort) ?? 'volume';
  set sort(String v) => _prefs.setString(_keySort, v);

  String get sidewaysAlgo => _prefs.getString(_keySidewaysAlgo) ?? 'v5';
  set sidewaysAlgo(String v) => _prefs.setString(_keySidewaysAlgo, v);

  String get sortDirection => _prefs.getString(_keySortDirection) ?? 'up';
  set sortDirection(String v) => _prefs.setString(_keySortDirection, v);

  bool get normalizeSparklines => _prefs.getBool(_keyNormalize) ?? true;
  set normalizeSparklines(bool v) => _prefs.setBool(_keyNormalize, v);

  bool get hiResSparklines => _prefs.getBool(_keyHiRes) ?? true;
  set hiResSparklines(bool v) => _prefs.setBool(_keyHiRes, v);

  bool get excludeStablecoins => _prefs.getBool(_keyExcludeStablecoins) ?? true;
  set excludeStablecoins(bool v) => _prefs.setBool(_keyExcludeStablecoins, v);

  // ---- event overlay settings ----

  bool get showEvents => _prefs.getBool(_keyShowEvents) ?? true;
  set showEvents(bool v) => _prefs.setBool(_keyShowEvents, v);

  /// Persisted as 'highOnly', 'highAndMedium', or 'all'. Default: 'highAndMedium'.
  String get eventFilter => _prefs.getString(_keyEventFilter) ?? 'highAndMedium';
  set eventFilter(String v) => _prefs.setString(_keyEventFilter, v);

  // ---- preferred exchange ----

  /// Persisted exchange key: 'binance', 'mexc', or 'bybit'. Default: 'binance'.
  String get preferredExchange => _prefs.getString(_keyPreferredExchange) ?? 'binance';
  set preferredExchange(String v) => _prefs.setString(_keyPreferredExchange, v);

  // ---- custom exchange ----

  /// User-defined custom exchange name, or null if not set.
  String? get customExchangeName => _prefs.getString(_keyCustomExchangeName);
  set customExchangeName(String? v) {
    if (v == null) {
      _prefs.remove(_keyCustomExchangeName);
    } else {
      _prefs.setString(_keyCustomExchangeName, v);
    }
  }

  /// User-defined custom exchange URL template, or null if not set.
  /// Should contain 'BTC' as placeholder for the base symbol.
  String? get customExchangeUrl => _prefs.getString(_keyCustomExchangeUrl);
  set customExchangeUrl(String? v) {
    if (v == null) {
      _prefs.remove(_keyCustomExchangeUrl);
    } else {
      _prefs.setString(_keyCustomExchangeUrl, v);
    }
  }

  // ---- macro events filters ----

  /// Selected countries for the macro events screen.
  /// Default: {'United States'}.
  Set<String> get selectedCountries =>
      (_prefs.getStringList(_keySelectedCountries) ?? ['United States']).toSet();

  set selectedCountries(Set<String> v) =>
      _prefs.setStringList(_keySelectedCountries, v.toList());

  /// Macro influence filter — set of impact levels to show.
  /// Stored as list of 'high', 'medium', 'low'. Default: all three.
  Set<String> get macroInfluenceFilter =>
      (_prefs.getStringList(_keyMacroInfluence) ?? ['high', 'medium', 'low']).toSet();

  set macroInfluenceFilter(Set<String> v) =>
      _prefs.setStringList(_keyMacroInfluence, v.toList());

  // ---- favourites ----

  Set<String> get favourites =>
      (_prefs.getStringList(_keyFavourites) ?? []).toSet();

  set favourites(Set<String> v) =>
      _prefs.setStringList(_keyFavourites, v.toList());

  bool isFavourite(String symbol) => favourites.contains(symbol);

  void addFavourite(String symbol) {
    final favs = favourites;
    favs.add(symbol);
    favourites = favs;
  }

  void removeFavourite(String symbol) {
    final favs = favourites;
    favs.remove(symbol);
    favourites = favs;
  }

  void toggleFavourite(String symbol) {
    if (isFavourite(symbol)) {
      removeFavourite(symbol);
    } else {
      addFavourite(symbol);
    }
  }

  // ---- social settings ----

  static const _keyShowSocialOnChart = 'settings.showSocialOnChart';
  static const _keyNotificationsEnabled = 'settings.notificationsEnabled';

  bool get hasSeenAbout => _prefs.getBool(_keyHasSeenAbout) ?? false;
  set hasSeenAbout(bool v) => _prefs.setBool(_keyHasSeenAbout, v);

  static const _keyNotifyMacroHigh = 'settings.notify.macroHigh';
  static const _keyNotifyMacroModerate = 'settings.notify.macroModerate';
  static const _keyNotifyUptrend = 'settings.notify.uptrend';
  static const _keyNotifyDowntrend = 'settings.notify.downtrend';
  static const _keyNotifySideways = 'settings.notify.sideways';
  static const _keyNotifySetupOfDay = 'settings.notify.setupOfDay';
  static const _keyNotifyNews = 'settings.notify.news';
  static const _keyUptrendMinDominance = 'settings.notify.uptrendMinDom';
  static const _keyDowntrendMinDominance = 'settings.notify.downtrendMinDom';
  static const _keySidewaysMinDominance = 'settings.notify.sidewaysMinDom';
  static const _keySetupMinScore = 'settings.notify.setupMinScore';
  static const _keyUptrendTimeframe = 'settings.notify.uptrendTf';
  static const _keyDowntrendTimeframe = 'settings.notify.downtrendTf';
  static const _keySidewaysTimeframe = 'settings.notify.sidewaysTf';
  static const _keySetupTimeframe = 'settings.notify.setupTf';
  static const _socialSettingsPrefix = 'social.settings.';

  bool get showSocialOnChart => _prefs.getBool(_keyShowSocialOnChart) ?? false;
  set showSocialOnChart(bool v) => _prefs.setBool(_keyShowSocialOnChart, v);

  bool get notificationsEnabled =>
      _prefs.getBool(_keyNotificationsEnabled) ?? false;
  set notificationsEnabled(bool v) =>
      _prefs.setBool(_keyNotificationsEnabled, v);

  bool get notifyMacroHigh => _prefs.getBool(_keyNotifyMacroHigh) ?? true;
  set notifyMacroHigh(bool v) => _prefs.setBool(_keyNotifyMacroHigh, v);

  bool get notifyMacroModerate => _prefs.getBool(_keyNotifyMacroModerate) ?? true;
  set notifyMacroModerate(bool v) => _prefs.setBool(_keyNotifyMacroModerate, v);

  bool get notifyUptrend => _prefs.getBool(_keyNotifyUptrend) ?? true;
  set notifyUptrend(bool v) => _prefs.setBool(_keyNotifyUptrend, v);

  bool get notifyDowntrend => _prefs.getBool(_keyNotifyDowntrend) ?? true;
  set notifyDowntrend(bool v) => _prefs.setBool(_keyNotifyDowntrend, v);

  bool get notifySideways => _prefs.getBool(_keyNotifySideways) ?? true;
  set notifySideways(bool v) => _prefs.setBool(_keyNotifySideways, v);

  bool get notifySetupOfDay => _prefs.getBool(_keyNotifySetupOfDay) ?? true;
  set notifySetupOfDay(bool v) => _prefs.setBool(_keyNotifySetupOfDay, v);

  bool get notifyNews => _prefs.getBool(_keyNotifyNews) ?? true;
  set notifyNews(bool v) => _prefs.setBool(_keyNotifyNews, v);

  double get uptrendMinDominance => _prefs.getDouble(_keyUptrendMinDominance) ?? 0.75;
  set uptrendMinDominance(double v) => _prefs.setDouble(_keyUptrendMinDominance, v);

  double get downtrendMinDominance => _prefs.getDouble(_keyDowntrendMinDominance) ?? 0.75;
  set downtrendMinDominance(double v) => _prefs.setDouble(_keyDowntrendMinDominance, v);

  double get sidewaysMinDominance => _prefs.getDouble(_keySidewaysMinDominance) ?? 0.75;
  set sidewaysMinDominance(double v) => _prefs.setDouble(_keySidewaysMinDominance, v);

  double get setupMinScore => _prefs.getDouble(_keySetupMinScore) ?? 0.75;
  set setupMinScore(double v) => _prefs.setDouble(_keySetupMinScore, v);

  String get uptrendTimeframe => _prefs.getString(_keyUptrendTimeframe) ?? '1h';
  set uptrendTimeframe(String v) => _prefs.setString(_keyUptrendTimeframe, v);

  String get downtrendTimeframe => _prefs.getString(_keyDowntrendTimeframe) ?? '1h';
  set downtrendTimeframe(String v) => _prefs.setString(_keyDowntrendTimeframe, v);

  String get sidewaysTimeframe => _prefs.getString(_keySidewaysTimeframe) ?? '1h';
  set sidewaysTimeframe(String v) => _prefs.setString(_keySidewaysTimeframe, v);

  String get setupTimeframe => _prefs.getString(_keySetupTimeframe) ?? '1h';
  set setupTimeframe(String v) => _prefs.setString(_keySetupTimeframe, v);

  /// Retrieves per-account filter settings for [handle].
  SocialAccountSettings getAccountSettings(String handle) {
    final raw = _prefs.getString('$_socialSettingsPrefix$handle');
    if (raw == null) return const SocialAccountSettings();
    return SocialAccountSettings.decode(raw);
  }

  /// Persists per-account filter settings for [handle].
  void setAccountSettings(String handle, SocialAccountSettings settings) {
    _prefs.setString('$_socialSettingsPrefix$handle', settings.encode());
  }
}
