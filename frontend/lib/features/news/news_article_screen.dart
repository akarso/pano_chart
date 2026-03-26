import 'package:flutter/material.dart';
import 'package:flutter_markdown/flutter_markdown.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../domain/news_article.dart';
import 'news_view_model.dart';

/// Screen displaying a single news article.
class NewsArticleScreen extends StatefulWidget {
  final NewsViewModel viewModel;
  final String slug;

  const NewsArticleScreen({
    Key? key,
    required this.viewModel,
    required this.slug,
  }) : super(key: key);

  @override
  State<NewsArticleScreen> createState() => _NewsArticleScreenState();
}

class _NewsArticleScreenState extends State<NewsArticleScreen> {
  @override
  void initState() {
    super.initState();
    widget.viewModel.onChanged = () {
      if (mounted) setState(() {});
    };
    widget.viewModel.loadArticle(widget.slug);
  }

  @override
  void dispose() {
    widget.viewModel.clearSelectedArticle();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final state = widget.viewModel.state;
    final article = state.selectedArticle;

    return Scaffold(
      backgroundColor: const Color(0xFF121212),
      appBar: AppBar(
        backgroundColor: const Color(0xFF1E1E1E),
        title: Text(article?.title ?? 'Loading...'),
        leading: IconButton(
          icon: const Icon(Icons.arrow_back),
          onPressed: () => Navigator.of(context).pop(),
        ),
      ),
      body: _buildBody(state, article),
    );
  }

  Widget _buildBody(NewsState state, NewsArticle? article) {
    if (state.isLoading && article == null) {
      return const Center(child: CircularProgressIndicator());
    }
    if (state.error != null && article == null) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.error_outline, color: Colors.grey, size: 48),
            const SizedBox(height: 12),
            Text(
              'Unable to load article',
              style: TextStyle(color: Colors.grey[400], fontSize: 16),
            ),
            const SizedBox(height: 8),
            TextButton(
              onPressed: () => widget.viewModel.loadArticle(widget.slug),
              child: const Text('Retry'),
            ),
          ],
        ),
      );
    }
    if (article == null) {
      return const Center(child: CircularProgressIndicator());
    }

    return SingleChildScrollView(
      padding: EdgeInsets.fromLTRB(
        20,
        20,
        20,
        20 + MediaQuery.of(context).padding.bottom,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Status + date header
          Row(
            children: [
              _StatusChip(status: article.status),
              const SizedBox(width: 12),
              Icon(Icons.calendar_today,
                  size: 14, color: Colors.grey[500]),
              const SizedBox(width: 4),
              Text(
                article.date,
                style: TextStyle(color: Colors.grey[500], fontSize: 13),
              ),
            ],
          ),
          if (article.eta != null) ...[
            const SizedBox(height: 8),
            Row(
              children: [
                Icon(Icons.schedule,
                    size: 14, color: Colors.grey[500]),
                const SizedBox(width: 4),
                Text(
                  'ETA: ${article.eta}',
                  style: TextStyle(color: Colors.grey[500], fontSize: 13),
                ),
              ],
            ),
          ],
          // Tags
          if (article.tags.isNotEmpty) ...[
            const SizedBox(height: 12),
            Wrap(
              spacing: 6,
              children: article.tags
                  .map((tag) => Chip(
                        label: Text(tag,
                            style: const TextStyle(fontSize: 11)),
                        backgroundColor: const Color(0xFF2A2A2A),
                        materialTapTargetSize:
                            MaterialTapTargetSize.shrinkWrap,
                        visualDensity: VisualDensity.compact,
                        padding: EdgeInsets.zero,
                      ))
                  .toList(),
            ),
          ],
          const SizedBox(height: 20),
          MarkdownBody(
            data: article.body,
            selectable: true,
            onTapLink: (text, href, title) {
              if (href != null) {
                launchUrl(Uri.parse(href),
                    mode: LaunchMode.externalApplication);
              }
            },
            styleSheet: MarkdownStyleSheet(
              p: const TextStyle(
                color: Colors.white70,
                fontSize: 15,
                height: 1.6,
              ),
              h1: const TextStyle(
                color: Colors.white,
                fontSize: 22,
                fontWeight: FontWeight.bold,
              ),
              h2: const TextStyle(
                color: Colors.white,
                fontSize: 19,
                fontWeight: FontWeight.bold,
              ),
              h3: const TextStyle(
                color: Colors.white,
                fontSize: 16,
                fontWeight: FontWeight.w600,
              ),
              listBullet: const TextStyle(
                color: Colors.white70,
                fontSize: 15,
              ),
              code: const TextStyle(
                color: Color(0xFF00E676),
                backgroundColor: Color(0xFF2A2A2A),
                fontSize: 13,
              ),
              codeblockDecoration: BoxDecoration(
                color: const Color(0xFF1E1E1E),
                borderRadius: BorderRadius.circular(8),
              ),
              blockquoteDecoration: BoxDecoration(
                border: Border(
                  left: BorderSide(
                    color: Colors.grey[600]!,
                    width: 3,
                  ),
                ),
              ),
              blockquotePadding:
                  const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
              a: const TextStyle(
                color: Color(0xFF00E676),
                decoration: TextDecoration.underline,
              ),
            ),
          ),
        ],
      ),
    );
  }

}

/// Colored status chip for the article detail view.
class _StatusChip extends StatelessWidget {
  final String status;

  const _StatusChip({required this.status});

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
      default:
        bgColor = const Color(0x33BDBDBD);
        textColor = const Color(0xFFBDBDBD);
    }

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
      decoration: BoxDecoration(
        color: bgColor,
        borderRadius: BorderRadius.circular(8),
      ),
      child: Text(
        status,
        style: TextStyle(
          color: textColor,
          fontSize: 12,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }
}
