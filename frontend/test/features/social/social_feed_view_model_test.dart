import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/social/api/social_account_settings.dart';
import 'package:pano_chart_frontend/features/social/api/social_api.dart';
import 'package:pano_chart_frontend/features/social/api/social_models.dart';
import 'package:pano_chart_frontend/features/social/social_feed_view_model.dart';

/// Recent timestamp helper — returns unix seconds for N hours ago.
int _recentTs(int hoursAgo) =>
    (DateTime.now().toUtc().subtract(Duration(hours: hoursAgo)).millisecondsSinceEpoch ~/ 1000);

class _FakeSocialApi implements SocialApi {
  SocialAccountsResponse accountsResult = const SocialAccountsResponse(
    userId: 'u1',
    count: 0,
    accounts: [],
  );

  final Map<String, SocialFeedResponse> feedResults = {};
  final List<String> subscribedHandles = [];
  final List<String> unsubscribedHandles = [];

  /// Captured settings per handle from the last fetchFeed call.
  final Map<String, SocialAccountSettings> capturedSettings = {};
  bool shouldThrow = false;

  @override
  Future<SocialAccountsResponse> fetchAccounts(String userId) async {
    if (shouldThrow) throw Exception('network error');
    return accountsResult;
  }

  @override
  Future<SocialFeedResponse> fetchFeed(
    String handle, {
    SocialAccountSettings settings = const SocialAccountSettings(),
  }) async {
    if (shouldThrow) throw Exception('network error');
    capturedSettings[handle] = settings;
    return feedResults[handle] ??
        SocialFeedResponse(handle: handle, count: 0, posts: const []);
  }

  @override
  Future<void> subscribe(
      {required String userId, required String handle}) async {
    if (shouldThrow) throw Exception('network error');
    subscribedHandles.add(handle);
  }

  @override
  Future<void> unsubscribe(
      {required String userId, required String handle}) async {
    if (shouldThrow) throw Exception('network error');
    unsubscribedHandles.add(handle);
  }

  @override
  Future<void> updateSettings({
    required String userId,
    required String handle,
    required SocialAccountSettings settings,
  }) async {
    // no-op for tests
  }
}

void main() {
  group('SocialFeedState', () {
    test('initial state', () {
      final state = SocialFeedState.initial();

      expect(state.isLoading, isFalse);
      expect(state.posts, isEmpty);
      expect(state.subscribedHandles, isEmpty);
      expect(state.error, isNull);
      expect(state.unseenCount, 0);
      expect(state.showOnChart, isFalse);
      expect(state.notificationsEnabled, isFalse);
    });

    test('copyWith preserves unchanged fields', () {
      final state = SocialFeedState.initial();
      final next = state.copyWith(isLoading: true);

      expect(next.isLoading, isTrue);
      expect(next.posts, isEmpty);
      expect(next.subscribedHandles, isEmpty);
      expect(next.unseenCount, 0);
    });

    test('copyWith clears error when not provided', () {
      const state = SocialFeedState(
        isLoading: false,
        posts: [],
        subscribedHandles: {},
        error: 'old error',
      );
      final next = state.copyWith(isLoading: true);

      expect(next.error, isNull);
    });
  });

  group('SocialFeedViewModel', () {
    test('initial state', () {
      final vm = SocialFeedViewModel(_FakeSocialApi(), userId: 'u1');

      expect(vm.state.isLoading, isFalse);
      expect(vm.state.posts, isEmpty);
      expect(vm.state.subscribedHandles, isEmpty);
      expect(vm.state.error, isNull);
    });

    test('loadSubscriptions sets handles and posts on success', () async {
      final ts = _recentTs(1);
      final fake = _FakeSocialApi()
        ..accountsResult = const SocialAccountsResponse(
          userId: 'u1',
          count: 1,
          accounts: ['twitter:elonmusk'],
        )
        ..feedResults['elonmusk'] = SocialFeedResponse(
          handle: 'elonmusk',
          count: 1,
          posts: [
            SocialPost(
              id: 'p1',
              accountId: 'twitter:elonmusk',
              author: 'elonmusk',
              title: 'Hello',
              url: 'https://x.com/1',
              timestamp: ts,
            ),
          ],
        );

      final vm = SocialFeedViewModel(fake, userId: 'u1');
      int notifications = 0;
      vm.onChanged = () => notifications++;

      await vm.loadSubscriptions();

      expect(vm.state.isLoading, isFalse);
      expect(vm.state.subscribedHandles, {'elonmusk'});
      expect(vm.state.posts.length, 1);
      expect(vm.state.posts[0].id, 'p1');
      expect(vm.state.error, isNull);
      expect(notifications, greaterThanOrEqualTo(2));
    });

    test('loadSubscriptions sets error on failure', () async {
      final fake = _FakeSocialApi()..shouldThrow = true;
      final vm = SocialFeedViewModel(fake, userId: 'u1');

      await vm.loadSubscriptions();

      expect(vm.state.isLoading, isFalse);
      expect(vm.state.error, isNotNull);
    });

    test('loadSubscriptions merges multiple feeds sorted newest first',
        () async {
      final tsOlder = _recentTs(4);
      final tsNewer = _recentTs(2);
      final fake = _FakeSocialApi()
        ..accountsResult = const SocialAccountsResponse(
          userId: 'u1',
          count: 2,
          accounts: ['twitter:alice', 'twitter:bob'],
        )
        ..feedResults['alice'] = SocialFeedResponse(
          handle: 'alice',
          count: 1,
          posts: [
            SocialPost(
              id: 'a1',
              accountId: 'twitter:alice',
              author: 'alice',
              title: 'Older',
              url: 'https://x.com/a1',
              timestamp: tsOlder,
            ),
          ],
        )
        ..feedResults['bob'] = SocialFeedResponse(
          handle: 'bob',
          count: 1,
          posts: [
            SocialPost(
              id: 'b1',
              accountId: 'twitter:bob',
              author: 'bob',
              title: 'Newer',
              url: 'https://x.com/b1',
              timestamp: tsNewer,
            ),
          ],
        );

      final vm = SocialFeedViewModel(fake, userId: 'u1');
      await vm.loadSubscriptions();

      expect(vm.state.posts.length, 2);
      expect(vm.state.posts[0].id, 'b1'); // newer first
      expect(vm.state.posts[1].id, 'a1');
    });

    test('subscribe calls API and adds handle', () async {
      final fake = _FakeSocialApi();
      final vm = SocialFeedViewModel(fake, userId: 'u1');
      int notifications = 0;
      vm.onChanged = () => notifications++;

      await vm.subscribe('newhandle');

      expect(fake.subscribedHandles, ['newhandle']);
      expect(vm.state.subscribedHandles, {'newhandle'});
      expect(notifications, greaterThanOrEqualTo(1));
    });

    test('subscribe sets error on failure', () async {
      final fake = _FakeSocialApi()..shouldThrow = true;
      final vm = SocialFeedViewModel(fake, userId: 'u1');

      await vm.subscribe('fail');

      expect(vm.state.error, isNotNull);
    });

    test('unsubscribe calls API and removes handle and posts', () async {
      final ts = _recentTs(1);
      final fake = _FakeSocialApi()
        ..accountsResult = const SocialAccountsResponse(
          userId: 'u1',
          count: 1,
          accounts: ['twitter:rem'],
        )
        ..feedResults['rem'] = SocialFeedResponse(
          handle: 'rem',
          count: 1,
          posts: [
            SocialPost(
              id: 'r1',
              accountId: 'twitter:rem',
              author: 'rem',
              title: 'Post',
              url: 'https://x.com/r1',
              timestamp: ts,
            ),
          ],
        );

      final vm = SocialFeedViewModel(fake, userId: 'u1');
      await vm.loadSubscriptions();
      expect(vm.state.subscribedHandles, {'rem'});
      expect(vm.state.posts.length, 1);

      await vm.unsubscribe('rem');

      expect(fake.unsubscribedHandles, ['rem']);
      expect(vm.state.subscribedHandles, isEmpty);
      expect(vm.state.posts, isEmpty);
    });

    test('unsubscribe sets error on failure', () async {
      final fake = _FakeSocialApi()..shouldThrow = true;
      final vm = SocialFeedViewModel(fake, userId: 'u1');

      await vm.unsubscribe('fail');

      expect(vm.state.error, isNotNull);
    });

    test('markAllSeen resets unseen count', () {
      final vm = SocialFeedViewModel(_FakeSocialApi(), userId: 'u1');
      int notifications = 0;
      vm.onChanged = () => notifications++;

      vm.markAllSeen();

      expect(vm.state.unseenCount, 0);
      expect(notifications, 1);
    });

    test('refreshFeeds updates posts', () async {
      final ts1 = _recentTs(3);
      final ts2 = _recentTs(1);
      final fake = _FakeSocialApi()
        ..accountsResult = const SocialAccountsResponse(
          userId: 'u1',
          count: 1,
          accounts: ['twitter:handle'],
        )
        ..feedResults['handle'] = SocialFeedResponse(
          handle: 'handle',
          count: 1,
          posts: [
            SocialPost(
              id: 'h1',
              accountId: 'twitter:handle',
              author: 'handle',
              title: 'First',
              url: 'https://x.com/h1',
              timestamp: ts1,
            ),
          ],
        );

      final vm = SocialFeedViewModel(fake, userId: 'u1');
      await vm.loadSubscriptions();
      expect(vm.state.posts.length, 1);

      // Update feed with new post
      fake.feedResults['handle'] = SocialFeedResponse(
        handle: 'handle',
        count: 2,
        posts: [
          SocialPost(
            id: 'h1',
            accountId: 'twitter:handle',
            author: 'handle',
            title: 'First',
            url: 'https://x.com/h1',
            timestamp: ts1,
          ),
          SocialPost(
            id: 'h2',
            accountId: 'twitter:handle',
            author: 'handle',
            title: 'Second',
            url: 'https://x.com/h2',
            timestamp: ts2,
          ),
        ],
      );

      await vm.refreshFeeds();

      expect(vm.state.posts.length, 2);
      // Unseen count should reflect 1 new post (h2)
      expect(vm.state.unseenCount, 1);
    });

    test('dispose stops polling', () {
      final vm = SocialFeedViewModel(_FakeSocialApi(), userId: 'u1');
      vm.startPolling();
      vm.dispose();
      // No error thrown — timer was cancelled.
    });

    test('loadSubscriptions with no accounts yields empty posts', () async {
      final fake = _FakeSocialApi()
        ..accountsResult = const SocialAccountsResponse(
          userId: 'u1',
          count: 0,
          accounts: [],
        );

      final vm = SocialFeedViewModel(fake, userId: 'u1');
      await vm.loadSubscriptions();

      expect(vm.state.subscribedHandles, isEmpty);
      expect(vm.state.posts, isEmpty);
      expect(vm.state.isLoading, isFalse);
    });

    test('partial feed failure still returns available posts', () async {
      final ts = _recentTs(1);
      final fake = _FakeSocialApi()
        ..accountsResult = const SocialAccountsResponse(
          userId: 'u1',
          count: 2,
          accounts: ['twitter:good', 'twitter:bad'],
        )
        ..feedResults['good'] = SocialFeedResponse(
          handle: 'good',
          count: 1,
          posts: [
            SocialPost(
              id: 'g1',
              accountId: 'twitter:good',
              author: 'good',
              title: 'Works',
              url: 'https://x.com/g1',
              timestamp: ts,
            ),
          ],
        );
      // 'bad' has no feed entry → default empty response

      final vm = SocialFeedViewModel(fake, userId: 'u1');
      await vm.loadSubscriptions();

      expect(vm.state.posts.length, 1);
      expect(vm.state.posts[0].id, 'g1');
    });

    // ── New PR-035 tests ──

    test('showOnChart toggle updates state', () {
      final vm = SocialFeedViewModel(_FakeSocialApi(), userId: 'u1');
      int notifications = 0;
      vm.onChanged = () => notifications++;

      expect(vm.showOnChart, isFalse);

      vm.showOnChart = true;
      expect(vm.showOnChart, isTrue);
      expect(vm.state.showOnChart, isTrue);
      expect(notifications, 1);
    });

    test('notificationsEnabled toggle updates state', () {
      final vm = SocialFeedViewModel(_FakeSocialApi(), userId: 'u1');
      int notifications = 0;
      vm.onChanged = () => notifications++;

      expect(vm.notificationsEnabled, isFalse);

      vm.notificationsEnabled = true;
      expect(vm.notificationsEnabled, isTrue);
      expect(vm.state.notificationsEnabled, isTrue);
      expect(notifications, 1);
    });

    test('onNewPost fires for brand-new posts when notifications enabled',
        () async {
      final ts = _recentTs(1);
      final fake = _FakeSocialApi()
        ..accountsResult = const SocialAccountsResponse(
          userId: 'u1',
          count: 1,
          accounts: ['twitter:a'],
        )
        ..feedResults['a'] = SocialFeedResponse(
          handle: 'a',
          count: 1,
          posts: [
            SocialPost(
              id: 'n1',
              accountId: 'twitter:a',
              author: 'a',
              title: 'New!',
              url: 'https://x.com/n1',
              timestamp: ts,
            ),
          ],
        );

      final vm = SocialFeedViewModel(fake, userId: 'u1');
      vm.notificationsEnabled = true;
      final notified = <String>[];
      vm.onNewPost = (p) => notified.add(p.id);

      await vm.loadSubscriptions();

      // First load — all posts are "new" but onNewPost fires only when
      // _knownIds was NOT empty (first load seeds _knownIds).
      expect(notified, isEmpty);

      // Now on refresh, add a truly new post.
      final ts2 = _recentTs(0);
      fake.feedResults['a'] = SocialFeedResponse(
        handle: 'a',
        count: 2,
        posts: [
          SocialPost(
            id: 'n1',
            accountId: 'twitter:a',
            author: 'a',
            title: 'New!',
            url: 'https://x.com/n1',
            timestamp: ts,
          ),
          SocialPost(
            id: 'n2',
            accountId: 'twitter:a',
            author: 'a',
            title: 'Brand new!',
            url: 'https://x.com/n2',
            timestamp: ts2,
          ),
        ],
      );

      await vm.refreshFeeds();
      expect(notified, ['n2']);
    });

    test('getSettings returns default when no prefs attached', () {
      final vm = SocialFeedViewModel(_FakeSocialApi(), userId: 'u1');
      final settings = vm.getSettings('handle');

      expect(settings.omitRetweets, isFalse);
      expect(settings.minLength, 0);
      expect(settings.keywords, isEmpty);
    });

    test('history prunes posts older than 7 days', () async {
      final recentTs = _recentTs(1);
      // 8 days ago — should be pruned.
      final oldTs = (DateTime.now()
                  .toUtc()
                  .subtract(const Duration(days: 8))
                  .millisecondsSinceEpoch ~/
              1000);

      final fake = _FakeSocialApi()
        ..accountsResult = const SocialAccountsResponse(
          userId: 'u1',
          count: 1,
          accounts: ['twitter:h'],
        )
        ..feedResults['h'] = SocialFeedResponse(
          handle: 'h',
          count: 2,
          posts: [
            SocialPost(
              id: 'old',
              accountId: 'twitter:h',
              author: 'h',
              title: 'Old',
              url: 'https://x.com/old',
              timestamp: oldTs,
            ),
            SocialPost(
              id: 'new',
              accountId: 'twitter:h',
              author: 'h',
              title: 'New',
              url: 'https://x.com/new',
              timestamp: recentTs,
            ),
          ],
        );

      final vm = SocialFeedViewModel(fake, userId: 'u1');
      await vm.loadSubscriptions();

      // Old post is pruned.
      expect(vm.state.posts.length, 1);
      expect(vm.state.posts[0].id, 'new');
    });

    test('isRetweet flag is preserved through feed loading', () async {
      final ts = _recentTs(1);
      final fake = _FakeSocialApi()
        ..accountsResult = const SocialAccountsResponse(
          userId: 'u1',
          count: 1,
          accounts: ['twitter:x'],
        )
        ..feedResults['x'] = SocialFeedResponse(
          handle: 'x',
          count: 1,
          posts: [
            SocialPost(
              id: 'rt1',
              accountId: 'twitter:x',
              author: 'x',
              title: 'RT @someone: hello',
              url: 'https://x.com/rt1',
              timestamp: ts,
              isRetweet: true,
            ),
          ],
        );

      final vm = SocialFeedViewModel(fake, userId: 'u1');
      await vm.loadSubscriptions();

      expect(vm.state.posts[0].isRetweet, isTrue);
    });

    test('updateSettings drops cached posts for affected handle before refresh',
        () async {
      final ts = _recentTs(1);
      final fake = _FakeSocialApi()
        ..accountsResult = const SocialAccountsResponse(
          userId: 'u1',
          count: 2,
          accounts: ['twitter:alice', 'twitter:bob'],
        )
        ..feedResults['alice'] = SocialFeedResponse(
          handle: 'alice',
          count: 2,
          posts: [
            SocialPost(
              id: 'a1',
              accountId: 'twitter:alice',
              author: 'alice',
              title: 'Short',
              url: 'https://x.com/a1',
              timestamp: ts,
            ),
            SocialPost(
              id: 'a2',
              accountId: 'twitter:alice',
              author: 'alice',
              title: 'A longer post that passes the filter',
              url: 'https://x.com/a2',
              timestamp: ts,
            ),
          ],
        )
        ..feedResults['bob'] = SocialFeedResponse(
          handle: 'bob',
          count: 1,
          posts: [
            SocialPost(
              id: 'b1',
              accountId: 'twitter:bob',
              author: 'bob',
              title: 'Bob post',
              url: 'https://x.com/b1',
              timestamp: ts,
            ),
          ],
        );

      final vm = SocialFeedViewModel(fake, userId: 'u1');
      await vm.loadSubscriptions();

      // All 3 posts loaded initially.
      expect(vm.state.posts.length, 3);

      // Now the backend will return only the longer alice post (simulating
      // server-side minLength filtering).
      fake.feedResults['alice'] = SocialFeedResponse(
        handle: 'alice',
        count: 1,
        posts: [
          SocialPost(
            id: 'a2',
            accountId: 'twitter:alice',
            author: 'alice',
            title: 'A longer post that passes the filter',
            url: 'https://x.com/a2',
            timestamp: ts,
          ),
        ],
      );

      // Apply settings — should drop alice's cached posts then refresh.
      vm.updateSettings(
        'alice',
        const SocialAccountSettings(minLength: 20),
      );

      // Let the async refreshFeeds complete.
      await Future<void>.delayed(Duration.zero);

      // 'a1' (short) should be gone; 'a2' + 'b1' remain.
      final ids = vm.state.posts.map((p) => p.id).toSet();
      expect(ids, containsAll(['a2', 'b1']));
      expect(ids.contains('a1'), isFalse,
          reason: 'stale unfiltered post should not survive settings change');
    });
  });
}
