import 'dart:convert';

import 'package:flutter_local_notifications/flutter_local_notifications.dart';

import 'api/social_models.dart';

/// Thin wrapper around [FlutterLocalNotificationsPlugin] for social feed alerts.
class NotificationService {
  final FlutterLocalNotificationsPlugin _plugin;
  bool _initialized = false;

  /// Called when the user taps a local notification. The argument is the
  /// JSON-decoded payload that was passed to [show].
  void Function(Map<String, dynamic> data)? onTap;

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
    await _plugin.initialize(
      settings,
      onDidReceiveNotificationResponse: _onNotificationTap,
    );
    _initialized = true;
  }

  void _onNotificationTap(NotificationResponse response) {
    final payload = response.payload;
    if (payload == null || payload.isEmpty) return;
    try {
      final data = json.decode(payload) as Map<String, dynamic>;
      onTap?.call(data);
    } catch (_) {
      // Malformed payload — ignore.
    }
  }

  /// Shows a local notification with the given [title] and [body].
  ///
  /// If [payload] is provided it is JSON-encoded and attached so that
  /// [onTap] receives it when the user taps the notification.
  Future<void> show({
    required String title,
    String? body,
    String channelId = 'general',
    String channelName = 'General',
    Map<String, dynamic>? payload,
  }) async {
    if (!_initialized) return;
    final androidDetails = AndroidNotificationDetails(
      channelId,
      channelName,
      importance: Importance.defaultImportance,
      priority: Priority.defaultPriority,
    );
    const iosDetails = DarwinNotificationDetails();
    final details = NotificationDetails(
      android: androidDetails,
      iOS: iosDetails,
    );
    await _plugin.show(
      title.hashCode ^ (body?.hashCode ?? 0),
      title,
      body,
      details,
      payload: payload != null ? json.encode(payload) : null,
    );
  }

  /// Shows a local notification for a new social post.
  Future<void> showNewPostNotification(SocialPost post) async {
    final handle = post.accountId.contains(':')
        ? post.accountId.split(':').last
        : post.accountId;
    await show(
      title: 'New post from @$handle',
      body: post.title,
      channelId: 'social_feed',
      channelName: 'Social Feed',
    );
  }
}
