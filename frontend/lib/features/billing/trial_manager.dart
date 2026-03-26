import 'package:shared_preferences/shared_preferences.dart';
import '../../core/analytics.dart';

/// Tracks the app's 14-day free trial period.
///
/// The install date is recorded on first access and never changes.
/// [hasAccess] returns `true` while fewer than [trialDays] have elapsed
/// since installation.
class TrialManager {
  /// Length of the free trial in days.
  static const int trialDays = 14;

  static const String _keyInstallDate = 'trial.installDate';
  static const String _keyExpiredLogged = 'trial.expiredLogged';

  final SharedPreferences _prefs;

  TrialManager(this._prefs) {
    _ensureInstallDate();
  }

  /// Visible for testing — allows creating a TrialManager whose install
  /// date was already written by a prior test setup.
  void _ensureInstallDate() {
    if (_prefs.getString(_keyInstallDate) == null) {
      _prefs.setString(
        _keyInstallDate,
        DateTime.now().toUtc().toIso8601String(),
      );
      Analytics().trialStarted();
    }
  }

  /// The date the app was first launched.
  DateTime get installDate =>
      DateTime.parse(_prefs.getString(_keyInstallDate)!);

  /// Whether the free trial is still active.
  bool isTrialActive([DateTime? now]) {
    final n = now ?? DateTime.now().toUtc();
    final active = n.difference(installDate).inDays < trialDays;
    if (!active && _prefs.getBool(_keyExpiredLogged) != true) {
      _prefs.setBool(_keyExpiredLogged, true);
      Analytics().trialExpired();
    }
    return active;
  }

  /// How many full days remain in the trial (clamped to 0..trialDays).
  int daysRemaining([DateTime? now]) {
    final n = now ?? DateTime.now().toUtc();
    final remaining = trialDays - n.difference(installDate).inDays;
    return remaining.clamp(0, trialDays);
  }
}
