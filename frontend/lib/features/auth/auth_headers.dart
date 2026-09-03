import 'package:http/http.dart' as http;

/// Builds the `Authorization` header for an authenticated backend request.
///
/// [getSecret] is a closure (not a raw string) so callers always read the
/// current secret at request time — it can change if the app re-claims a
/// fresh identity after a 401.
Map<String, String> authHeaders(
  String? Function()? getSecret, [
  Map<String, String>? extra,
]) {
  final headers = <String, String>{...?extra};
  final secret = getSecret?.call();
  if (secret != null && secret.isNotEmpty) {
    headers['Authorization'] = 'Bearer $secret';
  }
  return headers;
}

/// Performs an authenticated request via [send], retrying once with a
/// freshly claimed secret if the server responds 401.
///
/// [send] must perform the actual HTTP call using the given headers (which
/// already include `Authorization` plus [extraHeaders]). [reclaim] should
/// re-claim a device credential and persist it so the next call to
/// [getSecret] returns the new value — if it's null, or the retry still
/// gets a 401, the 401 response is simply returned to the caller.
///
/// [reclaim] MUST be single-flighted by the caller (see
/// `core/async/single_flight.dart`) if it can be shared across multiple
/// concurrent [sendAuthenticated] calls, which is the normal case (one
/// `reclaim` closure wired into every API client). Without that, a burst of
/// requests failing with 401 around the same moment each start their own
/// claim; only one can win (the backend's first-claim-wins rule), and the
/// losing callers retry with a secret that was never updated for them.
Future<http.Response> sendAuthenticated(
  Future<http.Response> Function(Map<String, String> headers) send,
  String? Function()? getSecret,
  Future<void> Function()? reclaim, [
  Map<String, String>? extraHeaders,
]) async {
  var response = await send(authHeaders(getSecret, extraHeaders));
  if (response.statusCode == 401 && reclaim != null) {
    await reclaim();
    response = await send(authHeaders(getSecret, extraHeaders));
  }
  return response;
}
