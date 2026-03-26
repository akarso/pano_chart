import 'social_account_settings.dart';
import 'social_models.dart';

/// Port for fetching social data from the backend.
abstract class SocialApi {
  /// Fetches the post feed for the given [handle], optionally filtered by
  /// [settings].
  Future<SocialFeedResponse> fetchFeed(
    String handle, {
    SocialAccountSettings settings,
  });

  /// Subscribes [userId] to [handle]. Idempotent.
  Future<void> subscribe({required String userId, required String handle});

  /// Unsubscribes [userId] from [handle].
  Future<void> unsubscribe({required String userId, required String handle});

  /// Returns the account IDs the given [userId] is subscribed to.
  Future<SocialAccountsResponse> fetchAccounts(String userId);

  /// Syncs per-account filter settings to the server for push filtering.
  Future<void> updateSettings({
    required String userId,
    required String handle,
    required SocialAccountSettings settings,
  });
}
