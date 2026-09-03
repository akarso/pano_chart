import 'package:flutter/material.dart';

import '../../domain/news_article.dart';
import 'news_view_model.dart';
import 'news_article_screen.dart';

/// News list screen displaying all news articles.
class NewsListScreen extends StatefulWidget {
  final NewsViewModel viewModel;

  const NewsListScreen({Key? key, required this.viewModel}) : super(key: key);

  @override
  State<NewsListScreen> createState() => _NewsListScreenState();
}

class _NewsListScreenState extends State<NewsListScreen> {
  @override
  void initState() {
    super.initState();
    widget.viewModel.onChanged = () {
      if (mounted) setState(() {});
    };
    widget.viewModel.loadArticles();
  }

  @override
  void dispose() {
    widget.viewModel.onChanged = null;
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final state = widget.viewModel.state;

    return Scaffold(
      backgroundColor: const Color(0xFF121212),
      appBar: AppBar(
        backgroundColor: const Color(0xFF1E1E1E),
        title: const Text('News & Updates'),
        leading: IconButton(
          icon: const Icon(Icons.arrow_back),
          onPressed: () => Navigator.of(context).pop(),
        ),
      ),
      body: _buildBody(state),
    );
  }

  Widget _buildBody(NewsState state) {
    if (state.isLoading && state.articles.isEmpty) {
      return const Center(child: CircularProgressIndicator());
    }
    if (state.error != null && state.articles.isEmpty) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.error_outline, color: Colors.grey, size: 48),
            const SizedBox(height: 12),
            Text(
              'Unable to load news',
              style: TextStyle(color: Colors.grey[400], fontSize: 16),
            ),
            const SizedBox(height: 8),
            TextButton(
              onPressed: () => widget.viewModel.loadArticles(),
              child: const Text('Retry'),
            ),
          ],
        ),
      );
    }
    if (state.articles.isEmpty) {
      return Center(
        child: Text(
          'No updates yet',
          style: TextStyle(color: Colors.grey[500]),
        ),
      );
    }

    return RefreshIndicator(
      onRefresh: () => widget.viewModel.loadArticles(),
      child: ListView.separated(
        padding: const EdgeInsets.symmetric(vertical: 12, horizontal: 16),
        itemCount: state.articles.length,
        separatorBuilder: (_, __) => const SizedBox(height: 8),
        itemBuilder: (context, index) {
          return _NewsCard(
            item: state.articles[index],
            onTap: () => _openArticle(state.articles[index].slug),
          );
        },
      ),
    );
  }

  void _openArticle(String slug) {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => NewsArticleScreen(
          viewModel: widget.viewModel,
          slug: slug,
        ),
      ),
    );
  }
}

/// A card displaying a news list item.
class _NewsCard extends StatelessWidget {
  final NewsListItem item;
  final VoidCallback onTap;

  const _NewsCard({required this.item, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return Card(
      color: const Color(0xFF1E1E1E),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
      child: InkWell(
        borderRadius: BorderRadius.circular(12),
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Expanded(
                    child: Text(
                      item.title,
                      style: const TextStyle(
                        fontWeight: FontWeight.w600,
                        fontSize: 16,
                        color: Colors.white,
                      ),
                    ),
                  ),
                  const SizedBox(width: 8),
                  _StatusBadge(status: item.status),
                ],
              ),
              const SizedBox(height: 8),
              Text(
                item.excerpt,
                style: TextStyle(color: Colors.grey[400], fontSize: 14),
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
              ),
              const SizedBox(height: 8),
              Row(
                children: [
                  Icon(Icons.calendar_today,
                      size: 14, color: Colors.grey[600]),
                  const SizedBox(width: 4),
                  Text(
                    item.date,
                    style:
                        TextStyle(color: Colors.grey[600], fontSize: 12),
                  ),
                  if (item.eta != null) ...[
                    const SizedBox(width: 12),
                    Icon(Icons.schedule,
                        size: 14, color: Colors.grey[600]),
                    const SizedBox(width: 4),
                    Text(
                      'ETA: ${item.eta}',
                      style: TextStyle(
                          color: Colors.grey[600], fontSize: 12),
                    ),
                  ],
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Status badge with color-coded background.
class _StatusBadge extends StatelessWidget {
  final String status;

  const _StatusBadge({required this.status});

  @override
  Widget build(BuildContext context) {
    Color bgColor;
    Color textColor;
    switch (status) {
      case 'planned':
        bgColor = const Color(0x33FFD600);
        textColor = const Color(0xFFFFD600);
        break;
      case 'released':
        bgColor = const Color(0x3300E676);
        textColor = const Color(0xFF00E676);
        break;
      default: // 'info' and others
        bgColor = const Color(0x33BDBDBD);
        textColor = const Color(0xFFBDBDBD);
    }

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: bgColor,
        borderRadius: BorderRadius.circular(6),
      ),
      child: Text(
        status,
        style: TextStyle(
          color: textColor,
          fontSize: 11,
          fontWeight: FontWeight.w500,
        ),
      ),
    );
  }
}
