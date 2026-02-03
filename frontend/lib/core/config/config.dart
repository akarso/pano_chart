/// Application configuration values. Provided via constructor injection.
class AppConfig {
  final String apiBaseUrl;
  final String flavor; // e.g. "dev" | "prod"

  const AppConfig({required this.apiBaseUrl, required this.flavor});
}
