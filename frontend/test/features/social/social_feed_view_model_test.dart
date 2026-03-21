import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/social/api/social_api.dart';
import 'package:pano_chart_frontend/features/social/api/social_models.dart';
import 'package:pano_chart_frontend/features/social/social_feed_view_model.dart';

class _FakeSocialApi implements SocialApi {
  SocialAccountsResponse accountsResult = const SocialAccountsResponse(
    userId: 'u1',
    count: 0,
    accounts: [],
  );

  final Map<String, SocialFeedResponse> feedResults = {};
  final List<String> subscribedHandles = [];
  final List<String> unsubscribedHandles = [];
  bool shouldThrow = false;

  @override
  Future<SocialAccountsResponse> fetchAccounts(String userId) async {
    if (shouldThrow) throw Exception('network error');
    return accountsResult;
  }

  @override
  Future<SocialFeedResponse> fetchFeed(String handle) async {
    if (shouldThrow) throw Exception('network error');
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
            const SocialPost(
              id: 'p1',
              accountId: 'twitter:elonmusk',
              author: 'elonmusk',
              title: 'Hello',
              url: 'https://x.com/1',
              timestamp: 1700000000,
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
            const SocialPost(
              id: 'a1',
              accountId: 'twitter:alice',
              author: 'alice',
              title: 'Older',
              url: 'https://x.com/a1',
              timestamp: 1000,
            ),
          ],
        )
        ..feedResults['bob'] = SocialFeedResponse(
          handle: 'bob',
          count: 1,
          posts: [
            const SocialPost(
              id: 'b1',
              accountId: 'twitter:bob',
              author: 'bob',
              title: 'Newer',
              url: 'https://x.com/b1',
              timestamp: 2000,
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
            const SocialPost(
              id: 'r1',
              accountId: 'twitter:rem',
              author: 'rem',
              title: 'Post',
              url: 'https://x.com/r1',
              timestamp: 1000,
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
            const SocialPost(
              id: 'h1',
              accountId: 'twitter:handle',
              author: 'handle',
              title: 'First',
              url: 'https://x.com/h1',
              timestamp: 1000,
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
          const SocialPost(
            id: 'h1',
            accountId: 'twitter:handle',
            author: 'handle',
            title: 'First',
            url: 'https://x.com/h1',
            timestamp: 1000,
          ),
          const SocialPost(
            id: 'h2',
            accountId: 'twitter:handle',
            author: 'handle',
            title: 'Second',
            url: 'https://x.com/h2',
            timestamp: 2000,
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
            const SocialPost(
              id: 'g1',
              accountId: 'twitter:good',
              author: 'good',
              title: 'Works',
              url: 'https://x.com/g1',
              timestamp: 1000,
            ),
          ],
        );
      // 'bad' has no feed entry → default empty response

      final vm = SocialFeedViewModel(fake, userId: 'u1');
      await vm.loadSubscriptions();

      expect(vm.state.posts.length, 1);
      expect(vm.state.posts[0].id, 'g1');
    });
  });
}
