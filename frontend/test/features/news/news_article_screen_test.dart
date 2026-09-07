import 'package:flutter/material.dart';
import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/domain/news_article.dart';
import 'package:pano_chart_frontend/features/news/application/get_news.dart';
import 'package:pano_chart_frontend/features/news/news_article_screen.dart';
import 'package:pano_chart_frontend/features/news/news_view_model.dart';

/// Regression test for PR-078's flutter_markdown → flutter_markdown_plus
/// migration: confirms the screen still builds and actually renders
/// formatted markdown (not just raw asterisks/hashes) via the new package.
class _FakeGetNews implements GetNews {
  final NewsArticle article;
  _FakeGetNews(this.article);

  @override
  Future<List<NewsListItem>> list({int limit = 20}) async => [];

  @override
  Future<NewsArticle> getBySlug(String slug) async => article;
}

void main() {
  testWidgets('NewsArticleScreen renders markdown body via flutter_markdown_plus',
      (WidgetTester tester) async {
    const article = NewsArticle(
      slug: 'test-article',
      title: 'Test Article',
      date: '2026-01-01',
      status: 'published',
      tags: [],
      body: '# Heading\n\nSome **bold** text and a [link](https://example.com).',
    );
    final vm = NewsViewModel(_FakeGetNews(article));

    await tester.pumpWidget(MaterialApp(
      home: NewsArticleScreen(viewModel: vm, slug: 'test-article'),
    ));
    await tester.pumpAndSettle();

    // The markdown widget itself built successfully (no exception during
    // pumpAndSettle), and the heading/body text rendered as actual text
    // nodes, not the raw "# Heading" markdown source.
    expect(find.byType(MarkdownBody), findsOneWidget);
    expect(find.text('Heading'), findsOneWidget);
    expect(find.textContaining('bold'), findsWidgets);
    expect(find.textContaining('# Heading'), findsNothing);

    // Explicitly unmount rather than relying on the test framework's own
    // end-of-test teardown to drive dispose() — this is what actually
    // exercises (and locks in) the dispose() fix below: clearSelectedArticle()
    // used to fire onChanged -> setState() on an element mid-unmount.
    await tester.pumpWidget(const SizedBox.shrink());
    expect(tester.takeException(), isNull);
  });
}
