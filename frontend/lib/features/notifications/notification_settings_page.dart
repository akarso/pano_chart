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

  const NotificationSettingsPage({
    super.key,
    required this.prefs,
    this.configApi,
  });

  @override
  State<NotificationSettingsPage> createState() =>
      _NotificationSettingsPageState();
}

class _NotificationSettingsPageState extends State<NotificationSettingsPage> {
  late NotificationSettings _settings;

  late final TextEditingController _uptrendCtl;
  late final TextEditingController _downtrendCtl;
  late final TextEditingController _sidewaysCtl;
  late final TextEditingController _setupCtl;

  @override
  void initState() {
    super.initState();
    _settings = NotificationSettings.fromPrefs(widget.prefs);
    _uptrendCtl = TextEditingController(
        text: _pct(_settings.uptrendMinDominance));
    _downtrendCtl = TextEditingController(
        text: _pct(_settings.downtrendMinDominance));
    _sidewaysCtl = TextEditingController(
        text: _pct(_settings.sidewaysMinDominance));
    _setupCtl =
        TextEditingController(text: _pct(_settings.setupMinScore));

    _fetchServerConfig();
  }

  @override
  void dispose() {
    _uptrendCtl.dispose();
    _downtrendCtl.dispose();
    _sidewaysCtl.dispose();
    _setupCtl.dispose();
    super.dispose();
  }

  /// Fetches server-side config and merges into local state.
  Future<void> _fetchServerConfig() async {
    final api = widget.configApi;
    if (api == null) return;
    try {
      final remote = await api.fetch(widget.prefs.userId);
      if (!mounted) return;
      setState(() {
        _settings = remote;
        _uptrendCtl.text = _pct(_settings.uptrendMinDominance);
        _downtrendCtl.text = _pct(_settings.downtrendMinDominance);
        _sidewaysCtl.text = _pct(_settings.sidewaysMinDominance);
        _setupCtl.text = _pct(_settings.setupMinScore);
      });
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

  static String _pct(double v) => '${(v * 100).round()}';

  double? _parsePct(String text) {
    final n = int.tryParse(text);
    if (n == null || n < 0 || n > 100) return null;
    return n / 100;
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Notifications')),
      body: ListView(
        children: [
          _sectionTitle('Social'),
          _toggle(
            'Twitter Alerts',
            _settings.social,
            (v) => _update(() => _settings.social = v),
          ),
          _sectionTitle('Market'),
          _toggle(
            'Uptrend',
            _settings.uptrend,
            (v) => _update(() => _settings.uptrend = v),
          ),
          _thresholdField(_uptrendCtl, 'min. dominance', (v) {
            _settings.uptrendMinDominance = v;
            _settings.save(widget.prefs);
            _syncToServer();
          }),
          _toggle(
            'Downtrend',
            _settings.downtrend,
            (v) => _update(() => _settings.downtrend = v),
          ),
          _thresholdField(_downtrendCtl, 'min. dominance', (v) {
            _settings.downtrendMinDominance = v;
            _settings.save(widget.prefs);
            _syncToServer();
          }),
          _toggle(
            'Sideways',
            _settings.sideways,
            (v) => _update(() => _settings.sideways = v),
          ),
          _thresholdField(_sidewaysCtl, 'min. dominance', (v) {
            _settings.sidewaysMinDominance = v;
            _settings.save(widget.prefs);
            _syncToServer();
          }),
          const SizedBox(height: 8),
          _toggle(
            'Best Setup (daily)',
            _settings.setupOfDay,
            (v) => _update(() => _settings.setupOfDay = v),
          ),
          _thresholdField(_setupCtl, 'min. quality score', (v) {
            _settings.setupMinScore = v;
            _settings.save(widget.prefs);
            _syncToServer();
          }),
          _sectionTitle('Macro'),
          _toggle(
            'Macro Events (30 min before)',
            _settings.macro,
            (v) => _update(() => _settings.macro = v),
          ),
          _sectionTitle('News'),
          _toggle(
            'News Alerts',
            _settings.news,
            (v) => _update(() => _settings.news = v),
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

  Widget _thresholdField(
    TextEditingController controller,
    String label,
    ValueChanged<double> onChanged,
  ) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16),
      child: Row(
        children: [
          const SizedBox(width: 48),
          Expanded(
            child: TextField(
              controller: controller,
              keyboardType: TextInputType.number,
              decoration: InputDecoration(
                labelText: label,
                suffixText: '%',
                isDense: true,
              ),
              onSubmitted: (text) {
                final v = _parsePct(text);
                if (v != null) onChanged(v);
              },
            ),
          ),
          const SizedBox(width: 16),
        ],
      ),
    );
  }
}
