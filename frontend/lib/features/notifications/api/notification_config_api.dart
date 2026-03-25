import '../notification_settings_model.dart';

/// Port for syncing per-user notification config with the backend.
abstract class NotificationConfigApi {
  /// Fetches the current config for [userId], or defaults if none saved.
  Future<NotificationSettings> fetch(String userId);

  /// Persists [settings] for [userId] on the backend.
  Future<void> save(String userId, NotificationSettings settings);
}
