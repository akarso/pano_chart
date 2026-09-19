import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

/// PR-085: forbid deprecated glossary words in user-facing string literals
/// under `lib/features/`.
///
/// Fails if any string literal contains (case-insensitive, whole word)
/// `prevalence` or `breadth`, unless that source line has a `// glossary-ok`
/// comment (e.g. JSON key access that must stay stable).
///
/// Comments are masked before literal scanning so banned words inside
/// `//` / `/* */` documentation never fail the test. String literals
/// (including those that contain comment-like text) are preserved.
void main() {
  test('lib/features string literals avoid deprecated glossary words', () {
    final featuresDir = Directory('lib/features');
    expect(featuresDir.existsSync(), isTrue,
        reason: 'run from frontend/ (lib/features missing)');

    final banned = RegExp(r'\b(prevalence|breadth)\b', caseSensitive: false);
    final tripleQuoted = RegExp(
      r"r?'''[\s\S]*?'''|"
      r'r?"""[\s\S]*?"""',
    );
    final singleLineQuoted = RegExp(
      r"""'(?:[^'\\]|\\.)*'|"""
      r'''"(?:[^"\\]|\\.)*"''',
    );

    final violations = <String>[];

    for (final entity in featuresDir.listSync(recursive: true)) {
      if (entity is! File || !entity.path.endsWith('.dart')) continue;
      final original = entity.readAsStringSync();
      final rel = entity.path.replaceAll('\\', '/');
      final lines = original.split('\n');
      final source = _maskComments(original);

      for (final match in tripleQuoted.allMatches(source)) {
        final literal = match.group(0)!;
        final startLine = _lineNumberAt(source, match.start);
        final endLine = _lineNumberAt(source, match.end - 1);
        if (_rangeHasGlossaryOk(lines, startLine, endLine)) continue;
        if (banned.hasMatch(_stripQuotes(literal))) {
          violations.add('$rel:$startLine: ${_preview(literal)}');
        }
      }

      final maskedLines = source.split('\n');
      for (var i = 0; i < maskedLines.length; i++) {
        if (i < lines.length && lines[i].contains('// glossary-ok')) continue;
        for (final match in singleLineQuoted.allMatches(maskedLines[i])) {
          final literal = match.group(0)!;
          if (banned.hasMatch(_stripQuotes(literal))) {
            violations.add('$rel:${i + 1}: ${_preview(literal)}');
          }
        }
      }
    }

    expect(
      violations,
      isEmpty,
      reason: 'Deprecated glossary words in string literals '
          '(add // glossary-ok only for intentional JSON keys):\n'
          '${violations.join('\n')}',
    );
  });
}

/// Replaces `//` and `/* */` comment regions with spaces (keeps newlines and
/// overall length) so literal scanners never see comment text. String
/// literals are copied through unchanged so their contents remain matchable.
String _maskComments(String source) {
  final out = StringBuffer();
  var i = 0;
  while (i < source.length) {
    // Line comment
    if (source.startsWith('//', i)) {
      while (i < source.length && source[i] != '\n') {
        out.write(' ');
        i++;
      }
      continue;
    }
    // Block comment
    if (source.startsWith('/*', i)) {
      out.write('  ');
      i += 2;
      while (i < source.length) {
        if (source.startsWith('*/', i)) {
          out.write('  ');
          i += 2;
          break;
        }
        out.write(source[i] == '\n' ? '\n' : ' ');
        i++;
      }
      continue;
    }
    // Raw / normal triple-quoted strings
    if (source.startsWith("r'''", i) ||
        source.startsWith('r"""', i) ||
        source.startsWith("'''", i) ||
        source.startsWith('"""', i)) {
      final raw = source[i] == 'r';
      if (raw) {
        out.write('r');
        i++;
      }
      final quote = source.substring(i, i + 3);
      out.write(quote);
      i += 3;
      while (i < source.length) {
        if (source.startsWith(quote, i)) {
          out.write(quote);
          i += 3;
          break;
        }
        out.write(source[i]);
        i++;
      }
      continue;
    }
    // Raw single-quoted / double-quoted (triples handled above)
    if (source.startsWith("r'", i) || source.startsWith('r"', i)) {
      final q = source[i + 1];
      out.write('r');
      out.write(q);
      i += 2;
      while (i < source.length && source[i] != q && source[i] != '\n') {
        out.write(source[i]);
        i++;
      }
      if (i < source.length && source[i] == q) {
        out.write(q);
        i++;
      }
      continue;
    }
    // Normal single / double quoted (with escapes)
    if (source[i] == "'" || source[i] == '"') {
      final q = source[i];
      out.write(q);
      i++;
      while (i < source.length && source[i] != '\n') {
        if (source[i] == '\\' && i + 1 < source.length) {
          out.write(source[i]);
          out.write(source[i + 1]);
          i += 2;
          continue;
        }
        out.write(source[i]);
        if (source[i] == q) {
          i++;
          break;
        }
        i++;
      }
      continue;
    }
    out.write(source[i]);
    i++;
  }
  return out.toString();
}

int _lineNumberAt(String source, int offset) {
  var line = 1;
  for (var i = 0; i < offset && i < source.length; i++) {
    if (source.codeUnitAt(i) == 0x0A) line++;
  }
  return line;
}

bool _rangeHasGlossaryOk(List<String> lines, int startLine, int endLine) {
  for (var i = startLine; i <= endLine; i++) {
    if (i - 1 < lines.length && lines[i - 1].contains('// glossary-ok')) {
      return true;
    }
  }
  return false;
}

String _stripQuotes(String literal) {
  var s = literal;
  if (s.startsWith('r')) s = s.substring(1);
  if (s.startsWith("'''") || s.startsWith('"""')) {
    return s.substring(3, s.length - 3);
  }
  if (s.startsWith("'") || s.startsWith('"')) {
    return s.substring(1, s.length - 1);
  }
  return s;
}

String _preview(String literal) {
  if (literal.length <= 80) return literal;
  return '${literal.substring(0, 77)}...';
}
