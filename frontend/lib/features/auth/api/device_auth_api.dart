/// A server-issued device credential (see `POST /api/device/claim`).
class DeviceClaim {
  final String userId;
  final String secret;

  const DeviceClaim({required this.userId, required this.secret});
}

/// Port for claiming a server-issued device identity — the lightweight
/// alternative to full account login. No password, nothing to remember;
/// the secret this returns just proves "this request is the same install
/// that made the earlier ones" to the backend.
abstract class DeviceAuthApi {
  /// Claims a new credential. Pass [existingUserId] to bind the new secret
  /// to a pre-existing locally-generated ID instead of minting a fresh one
  /// (preserves subscription/notification history for older installs).
  Future<DeviceClaim> claim({String? existingUserId});
}
