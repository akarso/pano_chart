import 'package:flutter_local_notifications/flutter_local_notifications.dart';

import 'api/social_models.dart';

/// Thin wrapper around [FlutterLocalNotificationsPlugin] for social feed alerts.
class NotificationService {
  final FlutterLocalNotificationsPlugin _plugin;
  bool _initialized = false;

  NotificationService()
      : _plugin = FlutterLocalNotificationsPlugin();

  /// Visible for testing — inject a custom plugin instance.
  NotificationService.withPlugin(this._plugin);

  /// Initialises the plugin. Safe to call multiple times.
  Future<void> init() async {
    if (_initialized) return;
    const android = AndroidInitializationSettings('@mipmap/ic_launcher');
    const ios = DarwinInitializationSettings();
    const settings = InitializationSettings(android: android, iOS: ios);
    await _plugin.initialize(settings);
    _initialized = true;
  }

  /// Shows a local notification for a new social post.
  Future<void> showNewPostNotification(SocialPost post) async {
    if (!_initialized) return;
    const androidDetails = AndroidNotificationDetails(
      'social_feed',
      'Social Feed',
      channelDescription: 'Notifications for new social posts',
      importance: Importance.defaultImportance,
      priority: Priority.defaultPriority,
    );
    const iosDetails = DarwinNotificationDetails();
    const details = NotificationDetails(
      android: androidDetails,
      iOS: iosDetails,
    );
    final handle = post.accountId.contains(':')
        ? post.accountId.split(':').last
        : post.accountId;
    await _plugin.show(
      post.id.hashCode,
      'New post from @$handle',
      post.title,
      details,
    );
  }
}
