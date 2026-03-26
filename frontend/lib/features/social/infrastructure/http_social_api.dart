import 'dart:convert';

import 'package:http/http.dart' as http;

import '../api/social_account_settings.dart';
import '../api/social_api.dart';
import '../api/social_models.dart';

/// Exception thrown by [HttpSocialApi] on non-200 responses.
class HttpSocialApiException implements Exception {
  final int statusCode;
  final String message;

  const HttpSocialApiException({
    required this.statusCode,
    required this.message,
  });

  @override
  String toString() => 'HttpSocialApiException($statusCode): $message';
}

/// HTTP adapter implementing [SocialApi] against the backend social endpoints.
class HttpSocialApi implements SocialApi {
  final http.Client client;
  final String baseUrl;

  HttpSocialApi({required this.client, required this.baseUrl});

  @override
  Future<SocialFeedResponse> fetchFeed(
    String handle, {
    SocialAccountSettings settings = const SocialAccountSettings(),
  }) async {
    final params = <String, String>{'handle': handle};
    if (settings.omitRetweets) params['omit_retweets'] = 'true';
    if (settings.omitReplies) params['omit_replies'] = 'true';
    if (settings.minLength > 0) {
      params['min_length'] = settings.minLength.toString();
    }
    if (settings.keywords.isNotEmpty) {
      params['keywords'] = settings.keywords.join(',');
    }

    final uri = Uri.parse(baseUrl).replace(
      path: '/api/social/feed',
      queryParameters: params,
    );

    final response =
        await client.get(uri).timeout(const Duration(seconds: 15));

    if (response.statusCode != 200) {
      throw HttpSocialApiException(
        statusCode: response.statusCode,
        message: 'Social feed error: ${response.statusCode}',
      );
    }

    final json = jsonDecode(response.body) as Map<String, dynamic>;
    return SocialFeedResponse.fromJson(json);
  }

  @override
  Future<void> subscribe(
      {required String userId, required String handle}) async {
    final uri = Uri.parse(baseUrl).replace(path: '/api/social/subscribe');
    final body = jsonEncode({'user_id': userId, 'handle': handle});

    final response = await client
        .post(uri, body: body, headers: {'Content-Type': 'application/json'})
        .timeout(const Duration(seconds: 15));

    if (response.statusCode != 201) {
      throw HttpSocialApiException(
        statusCode: response.statusCode,
        message: 'Subscribe error: ${response.statusCode}',
      );
    }
  }

  @override
  Future<void> unsubscribe(
      {required String userId, required String handle}) async {
    final uri = Uri.parse(baseUrl).replace(path: '/api/social/unsubscribe');
    final body = jsonEncode({'user_id': userId, 'handle': handle});

    final response = await client
        .post(uri, body: body, headers: {'Content-Type': 'application/json'})
        .timeout(const Duration(seconds: 15));

    if (response.statusCode != 200) {
      throw HttpSocialApiException(
        statusCode: response.statusCode,
        message: 'Unsubscribe error: ${response.statusCode}',
      );
    }
  }

  @override
  Future<SocialAccountsResponse> fetchAccounts(String userId) async {
    final uri = Uri.parse(baseUrl).replace(
      path: '/api/social/accounts',
      queryParameters: {'user_id': userId},
    );

    final response =
        await client.get(uri).timeout(const Duration(seconds: 15));

    if (response.statusCode != 200) {
      throw HttpSocialApiException(
        statusCode: response.statusCode,
        message: 'Social accounts error: ${response.statusCode}',
      );
    }

    final json = jsonDecode(response.body) as Map<String, dynamic>;
    return SocialAccountsResponse.fromJson(json);
  }

  @override
  Future<void> updateSettings({
    required String userId,
    required String handle,
    required SocialAccountSettings settings,
  }) async {
    final uri =
        Uri.parse(baseUrl).replace(path: '/api/social/subscribe/settings');
    final body = jsonEncode({
      'user_id': userId,
      'handle': handle,
      ...settings.toJson(),
    });

    final response = await client
        .put(uri, body: body, headers: {'Content-Type': 'application/json'})
        .timeout(const Duration(seconds: 15));

    if (response.statusCode != 200) {
      throw HttpSocialApiException(
        statusCode: response.statusCode,
        message: 'Update settings error: ${response.statusCode}',
      );
    }
  }
}
