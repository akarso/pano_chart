import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter/foundation.dart';
import 'package:flutter_local_notifications/flutter_local_notifications.dart';

import 'api/social_models.dart';
import '../notifications/sparkline_png.dart';

/// Renders sparkline closes to PNG bytes for notification big-picture style.
typedef SparklinePngRenderer = Future<Uint8List?> Function(List<double> points);

/// Thin wrapper around [FlutterLocalNotificationsPlugin] for social feed
/// alerts and contextual market / setup / watchlist pushes (PR-102).
class NotificationService {
  final FlutterLocalNotificationsPlugin _plugin;
  final SparklinePngRenderer _renderSparkline;
  bool _initialized = false;

  /// Called when the user taps a local notification. The argument is the
  /// JSON-decoded payload that was passed to [show].
  void Function(Map<String, dynamic> data)? onTap;

  NotificationService({SparklinePngRenderer? renderSparkline})
    : _plugin = FlutterLocalNotificationsPlugin(),
      _renderSparkline = renderSparkline ?? renderSparklinePng;

  /// Visible for testing — inject a custom plugin instance and optional
  /// sparkline renderer (e.g. one that throws to exercise fail-open).
  NotificationService.withPlugin(
    this._plugin, {
    SparklinePngRenderer? renderSparkline,
  }) : _renderSparkline = renderSparkline ?? renderSparklinePng;

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

  /// Marks the service ready without touching the real plugin (tests).
  @visibleForTesting
  void markInitializedForTest() => _initialized = true;

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

  /// Builds platform details. When [sparklinePng] is set, Android uses
  /// big-picture style with that image.
  @visibleForTesting
  static NotificationDetails buildDetails({
    String channelId = 'general',
    String channelName = 'General',
    String? title,
    String? body,
    Uint8List? sparklinePng,
  }) {
    final StyleInformation? style = sparklinePng == null
        ? null
        : BigPictureStyleInformation(
            ByteArrayAndroidBitmap(sparklinePng),
            contentTitle: title,
            summaryText: body,
            hideExpandedLargeIcon: true,
          );
    return NotificationDetails(
      android: AndroidNotificationDetails(
        channelId,
        channelName,
        importance: Importance.defaultImportance,
        priority: Priority.defaultPriority,
        styleInformation: style,
      ),
      iOS: const DarwinNotificationDetails(),
    );
  }

  /// Resolves sparkline bytes then builds details. Paint/encode failures
  /// yield plain details (styleInformation == null) so title/body still ship.
  /// PNG work is skipped on platforms that cannot show big-picture style
  /// (iOS / non-Android) — rendering would be discarded.
  @visibleForTesting
  Future<NotificationDetails> resolveDetails({
    String channelId = 'general',
    String channelName = 'General',
    String? title,
    String? body,
    List<double>? sparkline,
    Uint8List? sparklinePng,
  }) async {
    var png = sparklinePng;
    if (png == null &&
        sparkline != null &&
        sparkline.length >= 2 &&
        _supportsSparklineBigPicture) {
      try {
        png = await _renderSparkline(sparkline);
      } catch (_) {
        png = null;
      }
    }
    return buildDetails(
      channelId: channelId,
      channelName: channelName,
      title: title,
      body: body,
      sparklinePng: png,
    );
  }

  /// Android local notifications can attach a big-picture image; iOS
  /// [DarwinNotificationDetails] in this PR cannot, so skip the render.
  static bool get _supportsSparklineBigPicture =>
      !kIsWeb && defaultTargetPlatform == TargetPlatform.android;

  /// Shows a local notification with the given [title] and [body].
  ///
  /// If [payload] is provided it is JSON-encoded and attached so that
  /// [onTap] receives it when the user taps the notification.
  /// When [sparkline] has at least two points, Android uses a big-picture
  /// style with a rendered mini chart (PR-102). Paint/encode failures
  /// fall back to plain notification details so title/body still ship.
  Future<void> show({
    required String title,
    String? body,
    String channelId = 'general',
    String channelName = 'General',
    Map<String, dynamic>? payload,
    List<double>? sparkline,
    Uint8List? sparklinePng,
  }) async {
    if (!_initialized) return;

    final details = await resolveDetails(
      channelId: channelId,
      channelName: channelName,
      title: title,
      body: body,
      sparkline: sparkline,
      sparklinePng: sparklinePng,
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
