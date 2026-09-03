import 'dart:async';
import 'dart:ui' show VoidCallback;

import '../../infrastructure/preferences_service.dart';
import 'api/social_account_settings.dart';
import 'api/social_api.dart';
import 'api/social_models.dart';

/// Immutable state for the social feed feature.
class SocialFeedState {
  final bool isLoading;
  final List<SocialPost> posts;
  final Set<String> subscribedHandles;
  final String? error;
  final int unseenCount;

  /// Whether social posts should be shown on the detail chart.
  final bool showOnChart;

  /// Whether push-style notifications are enabled.
  final bool notificationsEnabled;

  const SocialFeedState({
    required this.isLoading,
    required this.posts,
    required this.subscribedHandles,
    this.error,
    this.unseenCount = 0,
    this.showOnChart = false,
    this.notificationsEnabled = false,
  });

  factory SocialFeedState.initial() => const SocialFeedState(
        isLoading: false,
        posts: [],
        subscribedHandles: {},
      );

  SocialFeedState copyWith({
    bool? isLoading,
    List<SocialPost>? posts,
    Set<String>? subscribedHandles,
    String? error,
    int? unseenCount,
    bool? showOnChart,
    bool? notificationsEnabled,
  }) {
    return SocialFeedState(
      isLoading: isLoading ?? this.isLoading,
      posts: posts ?? this.posts,
      subscribedHandles: subscribedHandles ?? this.subscribedHandles,
      error: error,
      unseenCount: unseenCount ?? this.unseenCount,
      showOnChart: showOnChart ?? this.showOnChart,
      notificationsEnabled: notificationsEnabled ?? this.notificationsEnabled,
    );
  }
}

/// ViewModel for the social feed feature.
///
/// Manages polling, subscriptions, per-account filter settings,
/// 7-day history accumulation, and feed state.
class SocialFeedViewModel {
  final SocialApi _api;
  final String _userId;

  VoidCallback? onChanged;

  /// Called when a brand-new post is detected. Can be used to trigger
  /// platform notifications.
  void Function(SocialPost post)? onNewPost;

  SocialFeedState _state = SocialFeedState.initial();
  SocialFeedState get state => _state;

  Timer? _pollTimer;
  static const _pollInterval = Duration(seconds: 60);

  /// 7-day history window for chart overlay data.
  static const _historyWindow = Duration(days: 7);

  PreferencesService? _prefs;

  /// IDs already seen — used for notification dedup.
  final Set<String> _knownIds = {};

  SocialFeedViewModel(this._api, {required String userId}) : _userId = userId;

  /// Attach preferences so settings persist across restarts.
  void attachPrefs(PreferencesService? prefs) {
    _prefs = prefs;
    if (prefs != null) {
      _state = _state.copyWith(
        showOnChart: prefs.showSocialOnChart,
        notificationsEnabled: prefs.notificationsEnabled,
      );
    }
  }

  // ── per-account settings ──

  SocialAccountSettings getSettings(String handle) {
    return _prefs?.getAccountSettings(handle) ?? const SocialAccountSettings();
  }

  void updateSettings(String handle, SocialAccountSettings settings) {
    _prefs?.setAccountSettings(handle, settings);
    // Drop cached posts for this handle so stale unfiltered data
    // does not survive the history merge in _loadFeeds.
    _state = _state.copyWith(
      posts: _state.posts.where((p) {
        final h =
            p.accountId.contains(':') ? p.accountId.split(':').last : p.accountId;
        return h != handle;
      }).toList(),
    );
    onChanged?.call();
    // Sync filter settings to server (fire-and-forget for push filtering).
    _api
        .updateSettings(userId: _userId, handle: handle, settings: settings)
        .catchError((_) {});
    refreshFeeds();
  }

  // ── chart toggle ──

  bool get showOnChart => _state.showOnChart;
  set showOnChart(bool v) {
    _prefs?.showSocialOnChart = v;
    _state = _state.copyWith(showOnChart: v);
    onChanged?.call();
  }

  // ── notifications toggle ──

  bool get notificationsEnabled => _state.notificationsEnabled;
  set notificationsEnabled(bool v) {
    _prefs?.notificationsEnabled = v;
    _state = _state.copyWith(notificationsEnabled: v);
    onChanged?.call();
  }

  /// Loads the user's subscribed accounts from the backend, then fetches
  /// all their feeds.
  Future<void> loadSubscriptions() async {
    _state = _state.copyWith(isLoading: true, error: null);
    onChanged?.call();

    try {
      final resp = await _api.fetchAccounts(_userId);
      // Account IDs come back as "twitter:handle" — extract handle.
      final handles = resp.accounts
          .map((id) => id.contains(':') ? id.split(':').last : id)
          .toSet();
      _state = _state.copyWith(subscribedHandles: handles);
      await _loadFeeds();
    } catch (e) {
      _state = _state.copyWith(isLoading: false, error: e.toString());
    }
    onChanged?.call();
  }

  /// Fetches feeds for all subscribed handles and merges into a single list
  /// sorted newest-first. Accumulates 7-day history for chart overlay and
  /// fires [onNewPost] for brand-new posts.
  Future<void> _loadFeeds() async {
    final handles = _state.subscribedHandles;
    if (handles.isEmpty) {
      _state = _state.copyWith(isLoading: false, posts: []);
      return;
    }

    final allPosts = <SocialPost>[];
    for (final handle in handles) {
      try {
        final settings = getSettings(handle);
        final resp = await _api.fetchFeed(handle, settings: settings);
        allPosts.addAll(resp.posts);
      } catch (_) {
        // Skip failed handles — partial data is acceptable.
      }
    }

    // Merge with existing history (de-duplicate by ID).
    final existingById = {for (final p in _state.posts) p.id: p};
    for (final p in allPosts) {
      existingById[p.id] = p;
    }

    // Prune posts older than 7 days.
    final cutoff = DateTime.now().toUtc().subtract(_historyWindow);
    final merged = existingById.values
        .where((p) => p.dateTime.isAfter(cutoff))
        .toList()
      ..sort((a, b) => b.timestamp.compareTo(a.timestamp));

    // Count truly new posts (IDs not previously known).
    final newPosts = allPosts.where((p) => !_knownIds.contains(p.id)).toList();
    final isFirstLoad = _knownIds.isEmpty;
    final unseen = isFirstLoad ? 0 : newPosts.length;

    // Fire notification callback for each new post (skip first load — seed only).
    if (!isFirstLoad && _state.notificationsEnabled && onNewPost != null) {
      for (final p in newPosts) {
        onNewPost!(p);
      }
    }

    // Track all known IDs.
    _knownIds.addAll(allPosts.map((p) => p.id));

    _state = _state.copyWith(
      isLoading: false,
      posts: merged,
      unseenCount: _state.unseenCount + unseen,
    );
  }

  /// Subscribes to a new handle. Calls the backend API then refreshes.
  Future<void> subscribe(String handle) async {
    try {
      await _api.subscribe(userId: _userId, handle: handle);
      final handles = Set<String>.from(_state.subscribedHandles)..add(handle);
      _state = _state.copyWith(subscribedHandles: handles);
      onChanged?.call();
      await refreshFeeds();
    } catch (e) {
      _state = _state.copyWith(error: e.toString());
      onChanged?.call();
    }
  }

  /// Unsubscribes from a handle.
  Future<void> unsubscribe(String handle) async {
    try {
      await _api.unsubscribe(userId: _userId, handle: handle);
      final handles = Set<String>.from(_state.subscribedHandles)
        ..remove(handle);
      _state = _state.copyWith(
        subscribedHandles: handles,
        posts: _state.posts.where((p) {
          final postHandle =
              p.accountId.contains(':') ? p.accountId.split(':').last : p.accountId;
          return postHandle != handle;
        }).toList(),
      );
      onChanged?.call();
    } catch (e) {
      _state = _state.copyWith(error: e.toString());
      onChanged?.call();
    }
  }

  /// Refreshes all feeds (can be called manually or by timer).
  Future<void> refreshFeeds() async {
    _state = _state.copyWith(isLoading: true, error: null);
    onChanged?.call();
    try {
      await _loadFeeds();
    } catch (e) {
      _state = _state.copyWith(isLoading: false, error: e.toString());
    }
    onChanged?.call();
  }

  /// Marks all current posts as "seen" (resets badge count).
  void markAllSeen() {
    _state = _state.copyWith(unseenCount: 0);
    onChanged?.call();
  }

  /// Starts the background polling timer.
  void startPolling() {
    _pollTimer?.cancel();
    _pollTimer = Timer.periodic(_pollInterval, (_) => refreshFeeds());
  }

  /// Stops the background polling timer.
  void stopPolling() {
    _pollTimer?.cancel();
    _pollTimer = null;
  }

  /// Cleans up resources.
  void dispose() {
    stopPolling();
  }
}
