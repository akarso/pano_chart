import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

import 'api/social_account_settings.dart';
import 'api/social_models.dart';
import 'social_feed_view_model.dart';

/// Popular accounts pre-filled for easy subscription.
const _popularAccounts = [
  'elonmusk',
  'realDonaldTrump',
  'binance',
  'VitalikButerin',
  'caborek05',
];

/// Screen displaying the social feed with account management.
class SocialFeedScreen extends StatefulWidget {
  final SocialFeedViewModel viewModel;

  const SocialFeedScreen({Key? key, required this.viewModel}) : super(key: key);

  @override
  State<SocialFeedScreen> createState() => _SocialFeedScreenState();
}

class _SocialFeedScreenState extends State<SocialFeedScreen>
    with WidgetsBindingObserver {
  final _handleController = TextEditingController();

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    widget.viewModel.onChanged = () {
      if (mounted) setState(() {});
    };
    widget.viewModel.markAllSeen();
    widget.viewModel.loadSubscriptions();
    widget.viewModel.startPolling();
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    widget.viewModel.stopPolling();
    widget.viewModel.onChanged = null;
    _handleController.dispose();
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.paused) {
      widget.viewModel.stopPolling();
    } else if (state == AppLifecycleState.resumed) {
      widget.viewModel.startPolling();
      widget.viewModel.refreshFeeds();
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = widget.viewModel.state;

    return Scaffold(
      backgroundColor: const Color(0xFF121212),
      appBar: AppBar(
        backgroundColor: const Color(0xFF1E1E1E),
        title: const Text('Social Feed'),
        leading: IconButton(
          icon: const Icon(Icons.arrow_back),
          onPressed: () => Navigator.of(context).pop(),
        ),
        actions: [
          IconButton(
            icon: const Icon(Icons.settings_outlined),
            tooltip: 'Feed settings',
            onPressed: () => _showSettingsSheet(context),
          ),
          IconButton(
            icon: const Icon(Icons.person_add_outlined),
            tooltip: 'Manage accounts',
            onPressed: () => _showAccountSheet(context),
          ),
        ],
      ),
      body: _buildBody(state),
    );
  }

  Widget _buildBody(SocialFeedState state) {
    if (state.isLoading && state.posts.isEmpty) {
      return const Center(child: CircularProgressIndicator());
    }
    if (state.error != null && state.posts.isEmpty) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.error_outline, color: Colors.grey, size: 48),
            const SizedBox(height: 12),
            Text(
              'Unable to load feed',
              style: TextStyle(color: Colors.grey[400], fontSize: 16),
            ),
            const SizedBox(height: 8),
            TextButton(
              onPressed: () => widget.viewModel.loadSubscriptions(),
              child: const Text('Retry'),
            ),
          ],
        ),
      );
    }
    if (state.subscribedHandles.isEmpty) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.rss_feed, color: Colors.grey[600], size: 64),
            const SizedBox(height: 16),
            Text(
              'No accounts tracked yet',
              style: TextStyle(color: Colors.grey[400], fontSize: 16),
            ),
            const SizedBox(height: 8),
            Text(
              'Tap + to add accounts',
              style: TextStyle(color: Colors.grey[600], fontSize: 14),
            ),
            const SizedBox(height: 16),
            FilledButton.icon(
              onPressed: () => _showAccountSheet(context),
              icon: const Icon(Icons.person_add),
              label: const Text('Add Account'),
            ),
          ],
        ),
      );
    }
    if (state.posts.isEmpty) {
      return Center(
        child: Text(
          'No posts yet',
          style: TextStyle(color: Colors.grey[500]),
        ),
      );
    }

    return RefreshIndicator(
      onRefresh: () => widget.viewModel.refreshFeeds(),
      child: ListView.separated(
        padding: const EdgeInsets.symmetric(vertical: 12, horizontal: 16),
        itemCount: state.posts.length,
        separatorBuilder: (_, __) => const SizedBox(height: 8),
        itemBuilder: (_, index) => _PostCard(post: state.posts[index]),
      ),
    );
  }

  void _showAccountSheet(BuildContext context) {
    showModalBottomSheet(
      context: context,
      backgroundColor: const Color(0xFF1E1E1E),
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
      ),
      isScrollControlled: true,
      builder: (_) => _AccountSheet(
        viewModel: widget.viewModel,
        handleController: _handleController,
        onFeedback: _showSnackBar,
      ),
    );
  }

  void _showSettingsSheet(BuildContext context) {
    showModalBottomSheet(
      context: context,
      backgroundColor: const Color(0xFF1E1E1E),
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
      ),
      builder: (_) => _FeedSettingsSheet(viewModel: widget.viewModel),
    );
  }

  void _showSnackBar(String message) {
    if (!mounted) return;
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(SnackBar(
        content: Text(message),
        behavior: SnackBarBehavior.floating,
        duration: const Duration(seconds: 2),
      ));
  }
}

// ── Post card ────────────────────────────────────────────────────────────────

class _PostCard extends StatelessWidget {
  final SocialPost post;

  const _PostCard({required this.post});

  @override
  Widget build(BuildContext context) {
    return Card(
      color: const Color(0xFF1E1E1E),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
      child: InkWell(
        borderRadius: BorderRadius.circular(12),
        onTap: () => _openUrl(post.url),
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Icon(Icons.person, size: 16, color: Colors.grey[500]),
                  const SizedBox(width: 6),
                  Text(
                    post.author,
                    style: const TextStyle(
                      fontWeight: FontWeight.w600,
                      fontSize: 13,
                      color: Color(0xFF00e6c0),
                    ),
                  ),
                  if (post.isRetweet) ...[
                    const SizedBox(width: 6),
                    Container(
                      padding: const EdgeInsets.symmetric(
                          horizontal: 4, vertical: 1),
                      decoration: BoxDecoration(
                        color: Colors.grey[800],
                        borderRadius: BorderRadius.circular(4),
                      ),
                      child: const Text(
                        'RT',
                        style: TextStyle(
                          fontSize: 9,
                          color: Colors.white54,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                    ),
                  ],
                  const Spacer(),
                  Text(
                    _formatTime(post.dateTime),
                    style: TextStyle(color: Colors.grey[600], fontSize: 12),
                  ),
                ],
              ),
              const SizedBox(height: 8),
              Text(
                post.title,
                style: const TextStyle(color: Colors.white, fontSize: 14),
                maxLines: 4,
                overflow: TextOverflow.ellipsis,
              ),
            ],
          ),
        ),
      ),
    );
  }

  Future<void> _openUrl(String url) async {
    final uri = Uri.tryParse(url);
    if (uri != null) {
      await launchUrl(uri, mode: LaunchMode.externalApplication);
    }
  }

  String _formatTime(DateTime dt) {
    final now = DateTime.now().toUtc();
    final diff = now.difference(dt);

    if (diff.inMinutes < 1) return 'just now';
    if (diff.inMinutes < 60) return '${diff.inMinutes}m ago';
    if (diff.inHours < 24) return '${diff.inHours}h ago';
    if (diff.inDays < 7) return '${diff.inDays}d ago';

    return '${dt.day}/${dt.month}/${dt.year}';
  }
}

// ── Feed-level settings sheet ───────────────────────────────────────────────

class _FeedSettingsSheet extends StatefulWidget {
  final SocialFeedViewModel viewModel;

  const _FeedSettingsSheet({required this.viewModel});

  @override
  State<_FeedSettingsSheet> createState() => _FeedSettingsSheetState();
}

class _FeedSettingsSheetState extends State<_FeedSettingsSheet> {
  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(16),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'Feed Settings',
            style: TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.w600,
              color: Colors.white,
            ),
          ),
          const SizedBox(height: 16),
          SwitchListTile(
            contentPadding: EdgeInsets.zero,
            title: const Text('Show on chart',
                style: TextStyle(color: Colors.white, fontSize: 14)),
            subtitle: Text('Display social posts on the detail chart',
                style: TextStyle(color: Colors.grey[600], fontSize: 12)),
            value: widget.viewModel.showOnChart,
            activeColor: const Color(0xFF42A5F5),
            onChanged: (v) {
              setState(() => widget.viewModel.showOnChart = v);
            },
          ),
          SwitchListTile(
            contentPadding: EdgeInsets.zero,
            title: const Text('Notifications',
                style: TextStyle(color: Colors.white, fontSize: 14)),
            subtitle: Text('Alert when new posts arrive',
                style: TextStyle(color: Colors.grey[600], fontSize: 12)),
            value: widget.viewModel.notificationsEnabled,
            activeColor: const Color(0xFF42A5F5),
            onChanged: (v) {
              setState(() => widget.viewModel.notificationsEnabled = v);
            },
          ),

          // Per-account settings (one row per subscribed handle).
          if (widget.viewModel.state.subscribedHandles.isNotEmpty) ...[
            const SizedBox(height: 8),
            Text(
              'Per-account filters',
              style: TextStyle(
                color: Colors.grey[500],
                fontSize: 12,
                fontWeight: FontWeight.w500,
              ),
            ),
            const SizedBox(height: 4),
            ...widget.viewModel.state.subscribedHandles.map((handle) {
              final settings = widget.viewModel.getSettings(handle);
              return ListTile(
                dense: true,
                contentPadding: EdgeInsets.zero,
                title: Text('@$handle',
                    style:
                        const TextStyle(color: Colors.white, fontSize: 14)),
                trailing: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    if (settings.hasActiveFilter)
                      const Icon(Icons.filter_alt,
                          size: 16, color: Color(0xFF42A5F5)),
                    IconButton(
                      icon: const Icon(Icons.tune,
                          size: 18, color: Colors.white54),
                      onPressed: () => _showAccountSettingsSheet(handle),
                    ),
                  ],
                ),
              );
            }),
          ],
        ],
      ),
    );
  }

  void _showAccountSettingsSheet(String handle) {
    showModalBottomSheet(
      context: context,
      backgroundColor: const Color(0xFF1E1E1E),
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
      ),
      isScrollControlled: true,
      builder: (_) => _AccountSettingsSheet(
        viewModel: widget.viewModel,
        handle: handle,
      ),
    );
  }
}

// ── Per-account settings sheet ──────────────────────────────────────────────

class _AccountSettingsSheet extends StatefulWidget {
  final SocialFeedViewModel viewModel;
  final String handle;

  const _AccountSettingsSheet({
    required this.viewModel,
    required this.handle,
  });

  @override
  State<_AccountSettingsSheet> createState() => _AccountSettingsSheetState();
}

class _AccountSettingsSheetState extends State<_AccountSettingsSheet> {
  late SocialAccountSettings _settings;
  final _keywordController = TextEditingController();

  @override
  void initState() {
    super.initState();
    _settings = widget.viewModel.getSettings(widget.handle);
  }

  @override
  void dispose() {
    _keywordController.dispose();
    super.dispose();
  }

  void _save() {
    widget.viewModel.updateSettings(widget.handle, _settings);
    Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    final bottomPadding = MediaQuery.of(context).viewInsets.bottom;

    return Padding(
      padding: EdgeInsets.only(
        left: 16,
        right: 16,
        top: 16,
        bottom: 16 + bottomPadding,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Filters for @${widget.handle}',
            style: const TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.w600,
              color: Colors.white,
            ),
          ),
          const SizedBox(height: 16),

          // Omit retweets toggle.
          SwitchListTile(
            contentPadding: EdgeInsets.zero,
            title: const Text('Omit retweets',
                style: TextStyle(color: Colors.white, fontSize: 14)),
            value: _settings.omitRetweets,
            activeColor: const Color(0xFF42A5F5),
            onChanged: (v) =>
                setState(() => _settings = _settings.copyWith(omitRetweets: v)),
          ),

          // Min body length.
          Row(
            children: [
              const Text('Min body length',
                  style: TextStyle(color: Colors.white, fontSize: 14)),
              const Spacer(),
              SizedBox(
                width: 60,
                child: TextField(
                  style: const TextStyle(color: Colors.white, fontSize: 14),
                  keyboardType: TextInputType.number,
                  textAlign: TextAlign.center,
                  decoration: InputDecoration(
                    hintText: '${_settings.minLength}',
                    hintStyle: TextStyle(color: Colors.grey[600]),
                    filled: true,
                    fillColor: const Color(0xFF2A2A2A),
                    border: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(8),
                      borderSide: BorderSide.none,
                    ),
                    contentPadding: const EdgeInsets.symmetric(
                        horizontal: 8, vertical: 6),
                  ),
                  onChanged: (v) {
                    final n = int.tryParse(v) ?? 0;
                    setState(
                        () => _settings = _settings.copyWith(minLength: n));
                  },
                ),
              ),
            ],
          ),

          const SizedBox(height: 12),

          // Keywords.
          Text('Keywords',
              style: TextStyle(color: Colors.grey[400], fontSize: 13)),
          const SizedBox(height: 6),
          Wrap(
            spacing: 6,
            runSpacing: 4,
            children: [
              ..._settings.keywords.map((kw) => Chip(
                    label: Text(kw,
                        style:
                            const TextStyle(color: Colors.white, fontSize: 12)),
                    backgroundColor: const Color(0xFF2A2A2A),
                    deleteIconColor: Colors.redAccent,
                    onDeleted: () {
                      final updated =
                          List<String>.from(_settings.keywords)..remove(kw);
                      setState(() =>
                          _settings = _settings.copyWith(keywords: updated));
                    },
                  )),
              if (_settings.keywords.length < 6)
                ActionChip(
                  label: const Icon(Icons.add, size: 16, color: Color(0xFF42A5F5)),
                  backgroundColor: const Color(0xFF2A2A2A),
                  onPressed: () => _showAddKeyword(),
                ),
            ],
          ),

          const SizedBox(height: 16),
          SizedBox(
            width: double.infinity,
            child: FilledButton(
              onPressed: _save,
              child: const Text('Apply'),
            ),
          ),
        ],
      ),
    );
  }

  void _showAddKeyword() {
    _keywordController.clear();
    showDialog(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: const Color(0xFF1E1E1E),
        title:
            const Text('Add Keyword', style: TextStyle(color: Colors.white)),
        content: TextField(
          controller: _keywordController,
          autofocus: true,
          style: const TextStyle(color: Colors.white),
          decoration: InputDecoration(
            hintText: 'e.g. bitcoin',
            hintStyle: TextStyle(color: Colors.grey[600]),
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(),
            child: const Text('Cancel'),
          ),
          TextButton(
            onPressed: () {
              final kw = _keywordController.text.trim();
              if (kw.isNotEmpty && !_settings.keywords.contains(kw)) {
                final updated = List<String>.from(_settings.keywords)..add(kw);
                setState(
                    () => _settings = _settings.copyWith(keywords: updated));
              }
              Navigator.of(ctx).pop();
            },
            child: const Text('Add'),
          ),
        ],
      ),
    );
  }
}

// ── Account management bottom sheet ─────────────────────────────────────────

class _AccountSheet extends StatefulWidget {
  final SocialFeedViewModel viewModel;
  final TextEditingController handleController;
  final void Function(String message) onFeedback;

  const _AccountSheet({
    required this.viewModel,
    required this.handleController,
    required this.onFeedback,
  });

  @override
  State<_AccountSheet> createState() => _AccountSheetState();
}

class _AccountSheetState extends State<_AccountSheet> {
  @override
  void initState() {
    super.initState();
    widget.viewModel.onChanged = () {
      if (mounted) setState(() {});
    };
  }

  Future<void> _subscribe(String handle) async {
    await widget.viewModel.subscribe(handle);
    widget.onFeedback('Subscribed to @$handle');
  }

  Future<void> _unsubscribe(String handle) async {
    await widget.viewModel.unsubscribe(handle);
    widget.onFeedback('Unsubscribed from @$handle');
  }

  @override
  Widget build(BuildContext context) {
    final subscribed = widget.viewModel.state.subscribedHandles;
    final bottomPadding = MediaQuery.of(context).viewInsets.bottom;

    return Padding(
      padding: EdgeInsets.only(
        left: 16,
        right: 16,
        top: 16,
        bottom: 16 + bottomPadding,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'Manage Accounts',
            style: TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.w600,
              color: Colors.white,
            ),
          ),
          const SizedBox(height: 16),

          // Custom handle input.
          Row(
            children: [
              Expanded(
                child: TextField(
                  controller: widget.handleController,
                  style: const TextStyle(color: Colors.white),
                  decoration: InputDecoration(
                    hintText: 'Enter handle (e.g. elonmusk)',
                    hintStyle: TextStyle(color: Colors.grey[600]),
                    prefixText: '@ ',
                    prefixStyle: TextStyle(color: Colors.grey[500]),
                    filled: true,
                    fillColor: const Color(0xFF2A2A2A),
                    border: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(8),
                      borderSide: BorderSide.none,
                    ),
                    contentPadding: const EdgeInsets.symmetric(
                        horizontal: 12, vertical: 10),
                  ),
                ),
              ),
              const SizedBox(width: 8),
              IconButton(
                icon: const Icon(Icons.add_circle, color: Color(0xFF00e6c0)),
                onPressed: () {
                  final handle = widget.handleController.text.trim();
                  if (handle.isNotEmpty) {
                    _subscribe(handle);
                    widget.handleController.clear();
                  }
                },
              ),
            ],
          ),

          const SizedBox(height: 16),
          Text(
            'Popular',
            style: TextStyle(
              color: Colors.grey[500],
              fontSize: 12,
              fontWeight: FontWeight.w500,
            ),
          ),
          const SizedBox(height: 8),

          // Popular accounts.
          ..._popularAccounts.map((handle) {
            final isSubscribed = subscribed.contains(handle);
            return ListTile(
              dense: true,
              contentPadding: EdgeInsets.zero,
              title: Text(
                '@$handle',
                style: const TextStyle(color: Colors.white, fontSize: 14),
              ),
              trailing: isSubscribed
                  ? IconButton(
                      icon: const Icon(Icons.remove_circle_outline,
                          color: Colors.redAccent, size: 20),
                      onPressed: () => _unsubscribe(handle),
                    )
                  : IconButton(
                      icon: const Icon(Icons.add_circle_outline,
                          color: Color(0xFF00e6c0), size: 20),
                      onPressed: () => _subscribe(handle),
                    ),
            );
          }),

          // Currently subscribed (non-popular).
          ...subscribed
              .where((h) => !_popularAccounts.contains(h))
              .map((handle) {
            return ListTile(
              dense: true,
              contentPadding: EdgeInsets.zero,
              title: Text(
                '@$handle',
                style: const TextStyle(color: Colors.white, fontSize: 14),
              ),
              trailing: IconButton(
                icon: const Icon(Icons.remove_circle_outline,
                    color: Colors.redAccent, size: 20),
                onPressed: () => _unsubscribe(handle),
              ),
            );
          }),
        ],
      ),
    );
  }
}
