import 'package:flutter/material.dart';

import '../../infrastructure/preferences_service.dart';
import 'api/notification_config_api.dart';
import 'notification_settings_model.dart';

/// Full-screen page for managing notification preferences per category.
///
/// Persists locally via [PreferencesService] and syncs to the backend
/// via [NotificationConfigApi] when provided.
class NotificationSettingsPage extends StatefulWidget {
  final PreferencesService prefs;
  final NotificationConfigApi? configApi;
  final bool isPro;

  const NotificationSettingsPage({
    super.key,
    required this.prefs,
    this.configApi,
    this.isPro = true,
  });

  @override
  State<NotificationSettingsPage> createState() =>
      _NotificationSettingsPageState();
}

class _NotificationSettingsPageState extends State<NotificationSettingsPage> {
  late NotificationSettings _settings;

  @override
  void initState() {
    super.initState();
    _settings = NotificationSettings.fromPrefs(widget.prefs);
    _fetchServerConfig();
  }

  /// Fetches server-side config and merges into local state.
  Future<void> _fetchServerConfig() async {
    final api = widget.configApi;
    if (api == null) return;
    try {
      final remote = await api.fetch(widget.prefs.userId);
      if (!mounted) return;
      setState(() => _settings = remote);
      _settings.save(widget.prefs);
    } catch (_) {
      // Use local values on network failure.
    }
  }

  void _update(void Function() mutate) {
    setState(mutate);
    _settings.save(widget.prefs);
    _syncToServer();
  }

  Future<void> _syncToServer() async {
    final api = widget.configApi;
    if (api == null) return;
    try {
      await api.save(widget.prefs.userId, _settings);
    } catch (_) {
      // Best-effort sync — local is always persisted first.
    }
  }

  @override
  Widget build(BuildContext context) {
    final isPro = widget.isPro;
    return Scaffold(
      appBar: AppBar(title: const Text('Notifications')),
      body: ListView(
        children: [
          // ── News (always visible) ──
          _sectionTitle('News'),
          _toggle(
            'News Alerts',
            _settings.news,
            (v) => _update(() => _settings.news = v),
          ),

          // ── Pro-only sections ──
          if (isPro) ...[
            _sectionTitle('Social'),
            _toggle(
              'Twitter Alerts',
              _settings.social,
              (v) => _update(() => _settings.social = v),
            ),
            _sectionTitle('Market'),
            _regimeRow(
              label: 'Uptrend',
              enabled: _settings.uptrend,
              onToggle: (v) => _update(() => _settings.uptrend = v),
              timeframe: _settings.uptrendTimeframe,
              onTimeframe: (v) =>
                  _update(() => _settings.uptrendTimeframe = v),
            ),
            _slider(
              'min. dominance',
              _settings.uptrendMinDominance,
              (v) => _update(() => _settings.uptrendMinDominance = v),
            ),
            _regimeRow(
              label: 'Downtrend',
              enabled: _settings.downtrend,
              onToggle: (v) => _update(() => _settings.downtrend = v),
              timeframe: _settings.downtrendTimeframe,
              onTimeframe: (v) =>
                  _update(() => _settings.downtrendTimeframe = v),
            ),
            _slider(
              'min. dominance',
              _settings.downtrendMinDominance,
              (v) => _update(() => _settings.downtrendMinDominance = v),
            ),
            _regimeRow(
              label: 'Sideways',
              enabled: _settings.sideways,
              onToggle: (v) => _update(() => _settings.sideways = v),
              timeframe: _settings.sidewaysTimeframe,
              onTimeframe: (v) =>
                  _update(() => _settings.sidewaysTimeframe = v),
            ),
            _slider(
              'min. dominance',
              _settings.sidewaysMinDominance,
              (v) => _update(() => _settings.sidewaysMinDominance = v),
            ),
            const SizedBox(height: 8),
            _regimeRow(
              label: 'Best Setup (daily)',
              enabled: _settings.setupOfDay,
              onToggle: (v) => _update(() => _settings.setupOfDay = v),
              timeframe: _settings.setupTimeframe,
              onTimeframe: (v) =>
                  _update(() => _settings.setupTimeframe = v),
            ),
            _slider(
              'min. quality score',
              _settings.setupMinScore,
              (v) => _update(() => _settings.setupMinScore = v),
            ),
            _sectionTitle('Macro'),
            _toggle(
              'Macro Events (30 min before)',
              _settings.macro,
              (v) => _update(() => _settings.macro = v),
            ),
          ],

          // ── Upgrade prompt for free users ──
          if (!isPro)
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 24, 16, 16),
              child: Text(
                'Upgrade to Pro for market, setup, macro, and social notifications.',
                style: TextStyle(
                  color: Colors.grey[400],
                  fontSize: 13,
                ),
              ),
            ),
        ],
      ),
    );
  }

  Widget _sectionTitle(String title) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 20, 16, 8),
      child: Text(
        title,
        style: const TextStyle(fontWeight: FontWeight.bold, fontSize: 13),
      ),
    );
  }

  Widget _toggle(String label, bool value, ValueChanged<bool> onChanged) {
    return SwitchListTile(
      title: Text(label),
      value: value,
      activeColor: const Color(0xFF42A5F5),
      onChanged: onChanged,
    );
  }

  /// A row combining a toggle switch with a timeframe dropdown.
  Widget _regimeRow({
    required String label,
    required bool enabled,
    required ValueChanged<bool> onToggle,
    required String timeframe,
    required ValueChanged<String> onTimeframe,
  }) {
    return Padding(
      padding: const EdgeInsets.only(right: 8),
      child: Row(
        children: [
          Expanded(
            child: SwitchListTile(
              title: Text(label),
              value: enabled,
              activeColor: const Color(0xFF42A5F5),
              onChanged: onToggle,
            ),
          ),
          _timeframeDropdown(timeframe, onTimeframe),
        ],
      ),
    );
  }

  Widget _timeframeDropdown(
    String value,
    ValueChanged<String> onChanged,
  ) {
    return DropdownButton<String>(
      value: value,
      underline: const SizedBox.shrink(),
      items: kTimeframes
          .map((tf) => DropdownMenuItem(value: tf, child: Text(tf)))
          .toList(),
      onChanged: (v) {
        if (v != null) onChanged(v);
      },
    );
  }

  Widget _slider(
    String label,
    double value,
    ValueChanged<double> onChanged,
  ) {
    final pct = (value * 100).round();
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16),
      child: Row(
        children: [
          const SizedBox(width: 48),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  '$label: $pct%',
                  style: const TextStyle(fontSize: 12),
                ),
                Slider(
                  value: value,
                  min: 0.50,
                  max: 1.0,
                  divisions: 10,
                  label: '$pct%',
                  onChanged: (v) => onChanged(v),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
