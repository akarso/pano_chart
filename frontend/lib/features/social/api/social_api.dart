import 'social_models.dart';

/// Port for fetching social data from the backend.
abstract class SocialApi {
  /// Fetches the post feed for the given [handle].
  Future<SocialFeedResponse> fetchFeed(String handle);

  /// Subscribes [userId] to [handle]. Idempotent.
  Future<void> subscribe({required String userId, required String handle});

  /// Unsubscribes [userId] from [handle].
  Future<void> unsubscribe({required String userId, required String handle});

  /// Returns the account IDs the given [userId] is subscribed to.
  Future<SocialAccountsResponse> fetchAccounts(String userId);
}
