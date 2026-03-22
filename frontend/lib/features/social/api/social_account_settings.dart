import 'dart:convert';

/// Per-account filter settings for a social feed subscription.
class SocialAccountSettings {
  final bool omitRetweets;
  final int minLength;
  final List<String> keywords;

  const SocialAccountSettings({
    this.omitRetweets = false,
    this.minLength = 0,
    this.keywords = const [],
  });

  SocialAccountSettings copyWith({
    bool? omitRetweets,
    int? minLength,
    List<String>? keywords,
  }) {
    return SocialAccountSettings(
      omitRetweets: omitRetweets ?? this.omitRetweets,
      minLength: minLength ?? this.minLength,
      keywords: keywords ?? this.keywords,
    );
  }

  Map<String, dynamic> toJson() => {
        'omit_retweets': omitRetweets,
        'min_length': minLength,
        'keywords': keywords,
      };

  factory SocialAccountSettings.fromJson(Map<String, dynamic> json) {
    return SocialAccountSettings(
      omitRetweets: json['omit_retweets'] as bool? ?? false,
      minLength: json['min_length'] as int? ?? 0,
      keywords: (json['keywords'] as List<dynamic>?)?.cast<String>() ?? const [],
    );
  }

  /// Serializes to a JSON string for SharedPreferences storage.
  String encode() => jsonEncode(toJson());

  /// Deserializes from a JSON string.
  factory SocialAccountSettings.decode(String source) {
    return SocialAccountSettings.fromJson(
      jsonDecode(source) as Map<String, dynamic>,
    );
  }

  /// Whether any filter is active.
  bool get hasActiveFilter => omitRetweets || minLength > 0 || keywords.isNotEmpty;
}
