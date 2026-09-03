/**
 * md-loader.js
 *
 * Lightweight markdown-article loader for section pages (blog, help, news …).
 *
 * Usage:
 *   <ul class="article-list">
 *     <li><a href="blog/My_Article.md">My Article Title</a></li>
 *   </ul>
 *
 * When a link is clicked the .md file is fetched, parsed into HTML and
 * rendered in an expandable <div> below the clicked header.  Clicking again
 * collapses it.  Only one article is expanded at a time.
 */

(function () {
  'use strict';

  // ── Tiny markdown → HTML converter ────────────────────────────────
  // Covers: headings, bold, italic, inline code, code blocks, links,
  // images, unordered/ordered lists, blockquotes, hr, paragraphs.

  function mdToHtml(md) {
    const lines = md.split('\n');
    const out = [];
    let i = 0;

    while (i < lines.length) {
      const line = lines[i];

      // Fenced code block
      if (line.trimStart().startsWith('```')) {
        const lang = line.trim().slice(3).trim();
        const codeLines = [];
        i++;
        while (i < lines.length && !lines[i].trimStart().startsWith('```')) {
          codeLines.push(escHtml(lines[i]));
          i++;
        }
        i++; // skip closing ```
        out.push(`<pre><code${lang ? ` class="language-${lang}"` : ''}>${codeLines.join('\n')}</code></pre>`);
        continue;
      }

      // Heading
      const hMatch = line.match(/^(#{1,6})\s+(.*)/);
      if (hMatch) {
        const lvl = hMatch[1].length;
        out.push(`<h${lvl}>${inline(hMatch[2])}</h${lvl}>`);
        i++;
        continue;
      }

      // Horizontal rule
      if (/^(\*{3,}|-{3,}|_{3,})\s*$/.test(line.trim())) {
        out.push('<hr>');
        i++;
        continue;
      }

      // Blockquote
      if (line.startsWith('>')) {
        const bqLines = [];
        while (i < lines.length && lines[i].startsWith('>')) {
          bqLines.push(lines[i].replace(/^>\s?/, ''));
          i++;
        }
        out.push(`<blockquote>${mdToHtml(bqLines.join('\n'))}</blockquote>`);
        continue;
      }

      // Unordered list
      if (/^\s*[-*+]\s/.test(line)) {
        const items = [];
        while (i < lines.length && /^\s*[-*+]\s/.test(lines[i])) {
          items.push(`<li>${inline(lines[i].replace(/^\s*[-*+]\s+/, ''))}</li>`);
          i++;
        }
        out.push(`<ul>${items.join('')}</ul>`);
        continue;
      }

      // Ordered list
      if (/^\s*\d+\.\s/.test(line)) {
        const items = [];
        while (i < lines.length && /^\s*\d+\.\s/.test(lines[i])) {
          items.push(`<li>${inline(lines[i].replace(/^\s*\d+\.\s+/, ''))}</li>`);
          i++;
        }
        out.push(`<ol>${items.join('')}</ol>`);
        continue;
      }

      // Blank line
      if (line.trim() === '') {
        i++;
        continue;
      }

      // Paragraph (collect consecutive non-blank lines)
      const para = [];
      while (i < lines.length && lines[i].trim() !== '' &&
             !/^#{1,6}\s/.test(lines[i]) &&
             !/^(\*{3,}|-{3,}|_{3,})\s*$/.test(lines[i].trim()) &&
             !/^\s*[-*+]\s/.test(lines[i]) &&
             !/^\s*\d+\.\s/.test(lines[i]) &&
             !lines[i].startsWith('>') &&
             !lines[i].trimStart().startsWith('```')) {
        para.push(lines[i]);
        i++;
      }
      out.push(`<p>${inline(para.join(' '))}</p>`);
    }

    return out.join('\n');
  }

  /** Inline markdown: bold, italic, code, links, images */
  function inline(text) {
    return text
      // images  ![alt](src)
      .replace(/!\[([^\]]*)\]\(([^)]+)\)/g, '<img src="$2" alt="$1">')
      // links   [text](url)
      .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>')
      // bold+italic  ***x***
      .replace(/\*{3}(.+?)\*{3}/g, '<strong><em>$1</em></strong>')
      // bold  **x**
      .replace(/\*{2}(.+?)\*{2}/g, '<strong>$1</strong>')
      // italic  *x*
      .replace(/\*(.+?)\*/g, '<em>$1</em>')
      // inline code  `x`
      .replace(/`([^`]+)`/g, '<code>$1</code>');
  }

  function escHtml(s) {
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  // ── Article list behaviour ────────────────────────────────────────

  /** Cache fetched markdown by URL so we don't re-fetch on toggle */
  const cache = {};

  function initArticleList() {
    document.querySelectorAll('.article-list a[href$=".md"]').forEach(link => {
      link.addEventListener('click', function (e) {
        e.preventDefault();
        toggleArticle(this);
      });
    });
  }

  function toggleArticle(link) {
    const li = link.closest('li');
    if (!li) return;

    // If this article is already open — collapse it
    const existing = li.querySelector('.article-body');
    if (existing) {
      existing.remove();
      li.classList.remove('open');
      return;
    }

    // Collapse any other open article in the same list
    const list = li.closest('.article-list');
    if (list) {
      list.querySelectorAll('.article-body').forEach(el => el.remove());
      list.querySelectorAll('li.open').forEach(el => el.classList.remove('open'));
    }

    const url = link.getAttribute('href');
    li.classList.add('open');

    if (cache[url]) {
      render(li, cache[url]);
      return;
    }

    // Show loading indicator
    const loader = document.createElement('div');
    loader.className = 'article-body loading';
    loader.textContent = 'Loading…';
    li.appendChild(loader);

    fetch(url)
      .then(r => {
        if (!r.ok) throw new Error(r.status);
        return r.text();
      })
      .then(md => {
        cache[url] = md;
        // Remove loader, render real content
        const old = li.querySelector('.article-body');
        if (old) old.remove();
        render(li, md);
      })
      .catch(() => {
        const old = li.querySelector('.article-body');
        if (old) {
          old.classList.remove('loading');
          old.innerHTML = '<p class="error">Could not load article.</p>';
        }
      });
  }

  function render(li, md) {
    const div = document.createElement('div');
    div.className = 'article-body';
    div.innerHTML = mdToHtml(md);
    li.appendChild(div);
  }

  // Boot
  function boot() {
    initArticleList();
    openFromHash();
    window.addEventListener('hashchange', openFromHash);
  }

  /** If the URL has a #fragment matching a li[data-anchor], expand it */
  function openFromHash() {
    const hash = location.hash.replace('#', '');
    if (!hash) return;
    const li = document.querySelector(`.article-list li[data-anchor="${hash}"]`);
    if (!li) return;
    // Already open — nothing to do
    if (li.classList.contains('open')) return;
    const link = li.querySelector('a[href$=".md"]');
    if (link) {
      toggleArticle(link);
      // Scroll into view after a short delay so the content renders first
      setTimeout(() => li.scrollIntoView({ behavior: 'smooth', block: 'start' }), 150);
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
})();
