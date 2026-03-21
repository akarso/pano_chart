import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

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
      ),
    );
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

// ── Account management bottom sheet ─────────────────────────────────────────

class _AccountSheet extends StatefulWidget {
  final SocialFeedViewModel viewModel;
  final TextEditingController handleController;

  const _AccountSheet({
    required this.viewModel,
    required this.handleController,
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
                    widget.viewModel.subscribe(handle);
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
                      onPressed: () => widget.viewModel.unsubscribe(handle),
                    )
                  : IconButton(
                      icon: const Icon(Icons.add_circle_outline,
                          color: Color(0xFF00e6c0), size: 20),
                      onPressed: () => widget.viewModel.subscribe(handle),
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
                onPressed: () => widget.viewModel.unsubscribe(handle),
              ),
            );
          }),
        ],
      ),
    );
  }
}
