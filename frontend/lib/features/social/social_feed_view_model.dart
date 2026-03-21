import 'dart:async';
import 'dart:ui' show VoidCallback;

import 'api/social_api.dart';
import 'api/social_models.dart';

/// Immutable state for the social feed feature.
class SocialFeedState {
  final bool isLoading;
  final List<SocialPost> posts;
  final Set<String> subscribedHandles;
  final String? error;
  final int unseenCount;

  const SocialFeedState({
    required this.isLoading,
    required this.posts,
    required this.subscribedHandles,
    this.error,
    this.unseenCount = 0,
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
  }) {
    return SocialFeedState(
      isLoading: isLoading ?? this.isLoading,
      posts: posts ?? this.posts,
      subscribedHandles: subscribedHandles ?? this.subscribedHandles,
      error: error,
      unseenCount: unseenCount ?? this.unseenCount,
    );
  }
}

/// ViewModel for the social feed feature.
///
/// Manages polling, subscriptions, and feed state.
class SocialFeedViewModel {
  final SocialApi _api;
  final String _userId;

  VoidCallback? onChanged;

  SocialFeedState _state = SocialFeedState.initial();
  SocialFeedState get state => _state;

  Timer? _pollTimer;
  static const _pollInterval = Duration(seconds: 60);

  SocialFeedViewModel(this._api, {required String userId}) : _userId = userId;

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
  /// sorted newest-first.
  Future<void> _loadFeeds() async {
    final handles = _state.subscribedHandles;
    if (handles.isEmpty) {
      _state = _state.copyWith(isLoading: false, posts: []);
      return;
    }

    final allPosts = <SocialPost>[];
    for (final handle in handles) {
      try {
        final resp = await _api.fetchFeed(handle);
        allPosts.addAll(resp.posts);
      } catch (_) {
        // Skip failed handles — partial data is acceptable.
      }
    }

    allPosts.sort((a, b) => b.timestamp.compareTo(a.timestamp));

    // Count truly new posts (IDs not in previous state).
    final previousIds = _state.posts.map((p) => p.id).toSet();
    final newCount =
        allPosts.where((p) => !previousIds.contains(p.id)).length;
    final unseen = _state.posts.isEmpty ? 0 : newCount;

    _state = _state.copyWith(
      isLoading: false,
      posts: allPosts,
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
