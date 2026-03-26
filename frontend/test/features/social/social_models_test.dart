import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/social/api/social_models.dart';

void main() {
  group('SocialPost', () {
    test('fromJson parses all fields', () {
      final json = {
        'id': 'p1',
        'account_id': 'twitter:elonmusk',
        'author': 'elonmusk',
        'title': 'Hello World',
        'url': 'https://x.com/elonmusk/status/123',
        'timestamp': 1700000000,
      };
      final post = SocialPost.fromJson(json);

      expect(post.id, 'p1');
      expect(post.accountId, 'twitter:elonmusk');
      expect(post.author, 'elonmusk');
      expect(post.title, 'Hello World');
      expect(post.url, 'https://x.com/elonmusk/status/123');
      expect(post.timestamp, 1700000000);
    });

    test('dateTime converts unix seconds to UTC DateTime', () {
      final post = SocialPost(
        id: 'p1',
        accountId: 'twitter:test',
        author: 'test',
        title: 'Title',
        url: 'https://example.com',
        timestamp: 1700000000,
      );
      final dt = post.dateTime;

      expect(dt.isUtc, isTrue);
      expect(dt, DateTime.utc(2023, 11, 14, 22, 13, 20));
    });

    test('isRetweet defaults to false when absent', () {
      final json = {
        'id': 'p1',
        'account_id': 'twitter:x',
        'author': 'x',
        'title': 'Hello',
        'url': 'https://x.com/1',
        'timestamp': 1700000000,
      };
      final post = SocialPost.fromJson(json);

      expect(post.isRetweet, isFalse);
    });

    test('isRetweet parsed from JSON', () {
      final json = {
        'id': 'rt1',
        'account_id': 'twitter:x',
        'author': 'x',
        'title': 'RT @someone: hello',
        'url': 'https://x.com/rt1',
        'timestamp': 1700000000,
        'is_retweet': true,
      };
      final post = SocialPost.fromJson(json);

      expect(post.isRetweet, isTrue);
    });

    test('isReply defaults to false when absent', () {
      final json = {
        'id': 'p1',
        'account_id': 'twitter:x',
        'author': 'x',
        'title': 'Hello',
        'url': 'https://x.com/1',
        'timestamp': 1700000000,
      };
      final post = SocialPost.fromJson(json);

      expect(post.isReply, isFalse);
    });

    test('isReply parsed from JSON', () {
      final json = {
        'id': 'r1',
        'account_id': 'twitter:x',
        'author': 'x',
        'title': '@someone nice post',
        'url': 'https://x.com/r1',
        'timestamp': 1700000000,
        'is_reply': true,
      };
      final post = SocialPost.fromJson(json);

      expect(post.isReply, isTrue);
    });
  });

  group('SocialFeedResponse', () {
    test('fromJson parses handle, count and posts', () {
      final json = {
        'handle': 'elonmusk',
        'count': 2,
        'posts': [
          {
            'id': 'p1',
            'account_id': 'twitter:elonmusk',
            'author': 'elonmusk',
            'title': 'Post One',
            'url': 'https://x.com/1',
            'timestamp': 1700000000,
          },
          {
            'id': 'p2',
            'account_id': 'twitter:elonmusk',
            'author': 'elonmusk',
            'title': 'Post Two',
            'url': 'https://x.com/2',
            'timestamp': 1700001000,
          },
        ],
      };
      final resp = SocialFeedResponse.fromJson(json);

      expect(resp.handle, 'elonmusk');
      expect(resp.count, 2);
      expect(resp.posts.length, 2);
      expect(resp.posts[0].id, 'p1');
      expect(resp.posts[1].id, 'p2');
    });

    test('fromJson handles null posts', () {
      final json = {'handle': 'nobody', 'count': 0};
      final resp = SocialFeedResponse.fromJson(json);

      expect(resp.posts, isEmpty);
    });

    test('fromJson handles empty posts', () {
      final json = {'handle': 'empty', 'count': 0, 'posts': []};
      final resp = SocialFeedResponse.fromJson(json);

      expect(resp.posts, isEmpty);
    });
  });

  group('SocialAccountsResponse', () {
    test('fromJson parses all fields', () {
      final json = {
        'user_id': 'u1',
        'count': 2,
        'accounts': ['twitter:elonmusk', 'twitter:binance'],
      };
      final resp = SocialAccountsResponse.fromJson(json);

      expect(resp.userId, 'u1');
      expect(resp.count, 2);
      expect(resp.accounts, ['twitter:elonmusk', 'twitter:binance']);
    });

    test('fromJson handles null accounts', () {
      final json = {'user_id': 'u1', 'count': 0};
      final resp = SocialAccountsResponse.fromJson(json);

      expect(resp.accounts, isEmpty);
    });

    test('fromJson handles empty accounts', () {
      final json = {'user_id': 'u1', 'count': 0, 'accounts': []};
      final resp = SocialAccountsResponse.fromJson(json);

      expect(resp.accounts, isEmpty);
    });
  });
}
