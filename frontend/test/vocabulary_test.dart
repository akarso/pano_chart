import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

/// PR-085: forbid deprecated glossary words in user-facing string literals
/// under `lib/features/`.
///
/// Fails if any string literal contains (case-insensitive, whole word)
/// `prevalence` or `breadth`, unless that source line has a `// glossary-ok`
/// comment (e.g. JSON key access that must stay stable).
///
/// Triple-quoted literals are scanned across the full file (multiline).
/// Single-line `'…'` / `"…"` stay line-oriented so apostrophes in comments
/// (e.g. `source's`) are not mistaken for string delimiters.
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
      final source = entity.readAsStringSync();
      final rel = entity.path.replaceAll('\\', '/');
      final lines = source.split('\n');

      for (final match in tripleQuoted.allMatches(source)) {
        final literal = match.group(0)!;
        final startLine = _lineNumberAt(source, match.start);
        final endLine = _lineNumberAt(source, match.end - 1);
        if (_rangeHasGlossaryOk(lines, startLine, endLine)) continue;
        if (banned.hasMatch(_stripQuotes(literal))) {
          violations.add('$rel:$startLine: ${_preview(literal)}');
        }
      }

      for (var i = 0; i < lines.length; i++) {
        final line = lines[i];
        if (line.contains('// glossary-ok')) continue;
        for (final match in singleLineQuoted.allMatches(line)) {
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
