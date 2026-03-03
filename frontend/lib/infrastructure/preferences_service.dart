import 'package:shared_preferences/shared_preferences.dart';

/// Persists user settings and favourites across app restarts.
///
/// Uses [SharedPreferences] as the backing store.
class PreferencesService {
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
  static const _keyFavourites = 'favourites';
  static const _keyShowEvents = 'settings.showEvents';
  static const _keyEventFilter = 'settings.eventFilter';
  static const _keyPreferredExchange = 'settings.preferredExchange';
  static const _keySelectedCountries = 'settings.selectedCountries';
  static const _keyMacroInfluence = 'settings.macroInfluenceFilter';

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

  String get sidewaysAlgo => _prefs.getString(_keySidewaysAlgo) ?? 'v1';
  set sidewaysAlgo(String v) => _prefs.setString(_keySidewaysAlgo, v);

  String get sortDirection => _prefs.getString(_keySortDirection) ?? 'up';
  set sortDirection(String v) => _prefs.setString(_keySortDirection, v);

  bool get normalizeSparklines => _prefs.getBool(_keyNormalize) ?? true;
  set normalizeSparklines(bool v) => _prefs.setBool(_keyNormalize, v);

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
}
