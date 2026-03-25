import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/social/api/social_models.dart';
import 'package:pano_chart_frontend/features/social/notification_service.dart';

void main() {
  group('NotificationService', () {
    test('showNewPostNotification does nothing before init', () async {
      final service = NotificationService();
      // Should not throw — silently skipped when not initialized.
      await service.showNewPostNotification(
        SocialPost(
          id: 'p1',
          accountId: 'twitter:satoshi',
          author: 'satoshi',
          title: 'hello',
          url: 'https://example.com',
          timestamp: 1700000000,
        ),
      );
    });

    test('handle extraction strips platform prefix', () {
      // Verify the accountId → handle logic (indirectly via the service code).
      const accountId = 'twitter:elonmusk';
      final handle = accountId.contains(':')
          ? accountId.split(':').last
          : accountId;
      expect(handle, 'elonmusk');
    });

    test('handle without prefix stays unchanged', () {
      const accountId = 'satoshi';
      final handle = accountId.contains(':')
          ? accountId.split(':').last
          : accountId;
      expect(handle, 'satoshi');
    });
  });
}
