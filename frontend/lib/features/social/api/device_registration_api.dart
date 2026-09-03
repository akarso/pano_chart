/// Port for registering/unregistering device push tokens with the backend.
abstract class DeviceRegistrationApi {
  /// Registers the device's FCM token with the backend.
  Future<void> register({
    required String userId,
    required String deviceId,
    required String fcmToken,
    required String platform,
  });

  /// Unregisters a device (e.g. on sign-out or token invalidation).
  Future<void> unregister({required String deviceId});
}
