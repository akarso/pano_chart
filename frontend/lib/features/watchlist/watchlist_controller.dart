import 'dart:async';

import 'package:flutter/foundation.dart';

import '../../infrastructure/preferences_service.dart';
import 'watchlist_api.dart';

/// Backend cap (`ports.WatchlistMaxSymbols`). The 51st symbol is rejected
/// locally so the client does not send a PUT the server will 400.
const kWatchlistMaxSymbols = 50;

const watchlistSyncFailedMessage = 'Could not update watchlist';
const watchlistRateLimitedMessage =
    'Too many watchlist updates. Wait a moment and try again.';
const watchlistCapMessage = 'Watchlist is full (50 symbols).';
const watchlistPendingMessage = 'Saved on this device. Not on the server yet.';

/// Shown when a stored symbol is rejected. The symbol is left out of the
/// next upload so one bad entry does not fail the rest of the list.
String watchlistInvalidSymbolMessage(String symbol) {
  if (symbol.isEmpty) {
    return 'A symbol on this watchlist is not valid and was skipped.';
  }
  return '$symbol is not a valid symbol and was skipped.';
}

/// Shown when several stored symbols are rejected in one upgrade.
String watchlistInvalidSymbolsMessage(Iterable<String> symbols) {
  final names = symbols.toList();
  if (names.length <= 1) {
    return watchlistInvalidSymbolMessage(names.isEmpty ? '' : names.single);
  }
  final shown = names.take(3).join(', ');
  final extra = names.length - 3;
  if (extra > 0) {
    return '$shown and $extra more are not valid symbols and were skipped.';
  }
  return '$shown are not valid symbols and were skipped.';
}

/// Shown when the upgrade keeps 50 symbols and drops the rest.
String watchlistLeftOffMessage(Iterable<String> dropped) {
  final names = dropped.toList();
  final shown = names.take(3).join(', ');
  final extra = names.length - 3;
  if (extra > 0) {
    return 'Watchlist keeps 50 symbols. Left off $shown and $extra more.';
  }
  return 'Watchlist keeps 50 symbols. Left off $shown.';
}

/// Shared watchlist membership for every screen that can star a symbol.
///
/// One instance lives in the composition root. Toggles update the local
/// cache immediately, then run one at a time on a single queue.
///
/// [serverSnapshotKnown] becomes true only after a fetch or a push
/// returns. Until then the controller does not PUT, because replace
/// deletes every server symbol missing from the body. A toggle made
/// while the snapshot is unknown stays pending and is merged after the
/// next successful fetch (startup, failure retry, or resume).
///
/// The first successful fetch for an install that is not yet migrated
/// unions the local cache with the server list, uploads that union
/// once (at most 50 symbols), and sets [PreferencesService.watchlistMigrated].
/// This device's stars fill the 50 slots before the server's. A failed
/// upload keeps pending adds and removals and does not set the flag, so
/// the next fetch retries the same union. A symbol is removed only when
/// the response names it as invalid. After the flag is set, an empty
/// server list means the user cleared it.
class WatchlistController extends ChangeNotifier {
  WatchlistController({PreferencesService? prefs, WatchlistApi? api})
    : this._(prefs, api);

  WatchlistController._(this._prefs, this._api) {
    _symbols = Set.of(_prefs?.favourites ?? const <String>{});
    final blocked = _prefs?.watchlistBlockedUpgrade;
    if (blocked != null) _blockedUpgrade = List.of(blocked);
    if (_prefs?.watchlistUpgradePending ?? false) {
      _status = watchlistPendingMessage;
    }
  }

  final PreferencesService? _prefs;
  final WatchlistApi? _api;

  Set<String> _symbols = {};

  /// Last list the server actually returned. Empty until
  /// [serverSnapshotKnown] is true — it is not a copy of the local cache.
  Set<String> _confirmed = {};

  final Set<String> _pendingAdds = {};
  final Set<String> _pendingRemoves = {};

  bool _serverKnown = false;
  int _generation = 0;

  /// A capped upgrade body the server rejected. The same body is not
  /// sent again, including after a process restart.
  List<String>? _blockedUpgrade;
  Future<void> _tail = Future<void>.value();
  bool _disposed = false;
  String? _status;

  Set<String> get symbols => Set.unmodifiable(_symbols);

  bool get serverSnapshotKnown => _serverKnown;

  /// Set when a toggle is rejected or rolled back. Cleared by [takeStatus].
  String? get statusMessage => _status;

  String? takeStatus() {
    final message = _status;
    _status = null;
    return message;
  }

  bool contains(String symbol) => _symbols.contains(symbol);

  /// Flips [symbol] locally, then syncs when a server snapshot exists.
  /// Returns false when the star was rejected or rolled back.
  /// [takeStatus] then holds the reason.
  Future<bool> toggle(String symbol) {
    if (symbol.isEmpty) {
      _status = watchlistInvalidSymbolMessage(symbol);
      _notify();
      return Future<bool>.value(false);
    }
    // Leave a pending upgrade sentence in place until _adopt clears it.
    final adding = !_symbols.contains(symbol);
    if (adding && _symbols.length >= kWatchlistMaxSymbols) {
      _status = watchlistCapMessage;
      _notify();
      return Future<bool>.value(false);
    }
    if (adding) {
      _prefs?.addFavourite(symbol);
      _symbols.add(symbol);
      _pendingRemoves.remove(symbol);
      _pendingAdds.add(symbol);
    } else {
      _prefs?.removeFavourite(symbol);
      _symbols.remove(symbol);
      _pendingAdds.remove(symbol);
      _pendingRemoves.add(symbol);
    }
    final gen = ++_generation;
    _notify();
    return _enqueueBool(() => _push(gen));
  }

  /// Fetches the server list, merges anything toggled while it was
  /// unknown, and pushes that merge. A failed fetch leaves the cache
  /// and the pending toggles in place.
  Future<void> reconcile() {
    return _enqueueVoid(() async {
      await _fetchAndMerge();
      if (_serverKnown) await _pushDiff(_generation, allowFollowUp: true);
    });
  }

  Future<bool> _push(int gen) async {
    if (gen != _generation) return true;
    if (_api == null) return true;
    if (!_serverKnown) {
      await _fetchAndMerge();
      if (gen != _generation) return true;
      if (!_serverKnown) {
        if (_status == null) {
          _status = watchlistPendingMessage;
          _notify();
        }
        return false;
      }
    }
    return _pushDiff(gen, allowFollowUp: true);
  }

  Future<void> _fetchAndMerge() async {
    final api = _api;
    if (api == null) return;
    final List<String> remote;
    try {
      remote = await api.fetch();
    } on TimeoutException {
      return;
    } on WatchlistRequestException {
      return;
    } on Exception {
      return;
    }

    final server = remote.toSet();
    final migrated = _prefs?.watchlistMigrated ?? false;
    if (!migrated) {
      await _migrate(api, server, remote);
      return;
    }

    final merged = Set<String>.of(server)
      ..removeAll(_pendingRemoves)
      ..addAll(_pendingAdds);
    _confirmed = Set<String>.of(server);
    _symbols = merged;
    _serverKnown = true;
    _pendingAdds.removeWhere(_confirmed.contains);
    _pendingRemoves.removeWhere((s) => !_confirmed.contains(s));
    _prefs?.favourites = _symbols;
    _notify();
  }

  /// First fetch for this install. Uploads local stars unioned with the
  /// server list, capped at 50, then remembers that the upgrade ran.
  Future<void> _migrate(
    WatchlistApi api,
    Set<String> server,
    List<String> serverOrder,
  ) async {
    final gen = _generation;
    var capped = _capForMigration(server, serverOrder);
    var payload = List<String>.of(capped.kept);
    if (_isBlocked(payload)) {
      _markPending();
      return;
    }
    if (capped.dropped.isNotEmpty) {
      _status = watchlistLeftOffMessage(capped.dropped);
    }
    // An empty equal list has nothing to upload. A non-empty equal list
    // is still sent once so a bad name already on the server can be
    // rejected and removed.
    if (_sameSymbols(payload, server) && payload.isEmpty) {
      _finishMigration(server);
      return;
    }

    final skipped = <String>[];
    var attempts = 0;
    while (attempts++ <= kWatchlistMaxSymbols) {
      try {
        final payloadSet = payload.toSet();
        // Replace drops rejected names without sending them. DELETE runs
        // the same normalize check and 400s on an invalid symbol.
        final onlyRemovals =
            skipped.isEmpty &&
            server.containsAll(payloadSet) &&
            payloadSet.length < server.length;
        final uploaded = onlyRemovals
            ? await api.remove(
                server.where((symbol) => !payloadSet.contains(symbol)).toList(),
              )
            : await api.replace(payload);
        if (gen != _generation) {
          _serverKnown = false;
          return;
        }
        _pendingAdds.clear();
        _pendingRemoves.clear();
        _clearBlockedUpgrade();
        _markMigrated();
        _adopt(uploaded.toSet());
        return;
      } on TimeoutException {
        await _recoverTimedOutMigration(api, payload, gen);
        return;
      } on WatchlistRequestException catch (error) {
        if (gen != _generation) return;
        if (_isTooMany(error)) {
          if (payload.length >= kWatchlistMaxSymbols) {
            _blockUpgrade(payload);
          }
          _serverKnown = false;
          _status = watchlistCapMessage;
          _markPending();
          return;
        }
        final bad = _invalidSymbol(error);
        // Named even when absent from the payload (cap left it off, then
        // DELETE re-checked it). Drop it and replace the cleaned body.
        if (bad != null) {
          _symbols.remove(bad);
          _pendingAdds.remove(bad);
          _prefs?.removeFavourite(bad);
          skipped.add(bad);
          capped = _capForMigration(server, serverOrder, exclude: skipped);
          payload = List<String>.of(capped.kept);
          if (payload.any(skipped.contains)) {
            payload = payload.where((s) => !skipped.contains(s)).toList();
          }
          final keptServer = server.difference(skipped.toSet());
          _status = _upgradeStatus(capped.dropped, skipped);
          // Server still holds the rejected name — replace the cleaned
          // body so that name is dropped without being sent again.
          if (skipped.any(server.contains)) {
            continue;
          }
          if (payload.isEmpty) {
            _finishMigration(keptServer);
            return;
          }
          if (_sameSymbols(payload, keptServer)) {
            _finishMigration(Set<String>.of(payload));
            return;
          }
          continue;
        }
        if (error.statusCode == 400) {
          _blockUpgrade(payload);
          _serverKnown = false;
          _markPending();
          return;
        }
        _rejectPendingUpload(error);
        return;
      } on Exception {
        if (gen != _generation) return;
        _rejectPendingUpload(null);
        return;
      }
    }
  }

  _CappedWatchlist _capForMigration(
    Set<String> server,
    List<String> serverOrder, {
    Iterable<String> exclude = const [],
  }) {
    final skip = exclude.toSet();
    final union = Set<String>.of(_symbols)
      ..addAll(server)
      ..removeAll(_pendingRemoves)
      ..removeAll(skip);
    final order = [
      for (final symbol in serverOrder)
        if (!skip.contains(symbol)) symbol,
    ];
    return _capUnion(union, order);
  }

  /// The upgrade call timed out. If the server applied that body, keep
  /// it. Otherwise leave the pending union in place for the next fetch.
  Future<void> _recoverTimedOutMigration(
    WatchlistApi api,
    List<String> payload,
    int gen,
  ) async {
    if (gen != _generation) {
      _serverKnown = false;
      return;
    }
    final List<String> remote;
    try {
      remote = await api.fetch();
    } on Exception {
      _serverKnown = false;
      _markPending();
      return;
    }
    if (gen != _generation) {
      _serverKnown = false;
      return;
    }
    if (_sameSymbols(payload, remote.toSet())) {
      _pendingAdds.clear();
      _pendingRemoves.clear();
      _clearBlockedUpgrade();
      _markMigrated();
      _adopt(remote.toSet());
      return;
    }
    _serverKnown = false;
    _markPending();
  }

  _CappedWatchlist _capUnion(Set<String> union, List<String> serverOrder) {
    final kept = <String>[];
    void take(Iterable<String> source) {
      for (final symbol in source) {
        if (kept.length >= kWatchlistMaxSymbols) return;
        if (!union.contains(symbol) || kept.contains(symbol)) continue;
        kept.add(symbol);
      }
    }

    // This device's pending adds and cached stars fill the cap before
    // symbols that exist only on the server.
    take(_pendingAdds);
    take(_symbols);
    take(serverOrder);
    take(union);
    final dropped = <String>[
      for (final symbol in _symbols)
        if (union.contains(symbol) && !kept.contains(symbol)) symbol,
      for (final symbol in union)
        if (!kept.contains(symbol) && !_symbols.contains(symbol)) symbol,
    ];
    return _CappedWatchlist(kept, dropped);
  }

  bool _sameSymbols(List<String> payload, Set<String> server) {
    if (payload.length != server.length) return false;
    return server.containsAll(payload);
  }

  void _finishMigration(Set<String> next) {
    _pendingAdds.clear();
    _pendingRemoves.clear();
    _clearBlockedUpgrade();
    _markMigrated();
    _adopt(next);
  }

  /// Pushes the difference between the local set and [_confirmed].
  /// A stale response is dropped and the snapshot is marked unknown so
  /// the next call refetches instead of diffing against that body.
  Future<bool> _pushDiff(int gen, {required bool allowFollowUp}) async {
    if (gen != _generation || !_serverKnown) return true;
    final api = _api;
    if (api == null) return true;

    final desired = Set<String>.of(_symbols);
    final added = desired.difference(_confirmed);
    final removed = _confirmed.difference(desired);
    if (added.isEmpty && removed.isEmpty) return true;

    try {
      final result = added.isNotEmpty
          ? await api.replace(desired.toList())
          : await api.remove(removed.toList());
      if (gen != _generation) {
        _serverKnown = false;
        return true;
      }
      _pendingAdds.removeAll(added);
      _pendingRemoves.removeAll(removed);
      _adopt(result.toSet());
      return true;
    } on TimeoutException {
      if (gen != _generation) {
        _serverKnown = false;
        return true;
      }
      _serverKnown = false;
      if (!allowFollowUp) {
        _markPending();
        return false;
      }
      await _fetchAndMerge();
      if (gen != _generation) return true;
      if (_editsOnServer()) {
        _clearPendingFromStatus();
        return true;
      }
      if (!_serverKnown) {
        _markPending();
        return false;
      }
      final ok = await _pushDiff(gen, allowFollowUp: false);
      if (gen != _generation) return true;
      // The timed-out call can still be applied after the follow-up.
      await _fetchAndMerge();
      if (gen != _generation) return true;
      if (_editsOnServer()) {
        _clearPendingFromStatus();
        return true;
      }
      if (!ok && _status != null && _status != watchlistPendingMessage) {
        return false;
      }
      _markPending();
      return false;
    } on WatchlistRequestException catch (error) {
      if (gen != _generation) return true;
      _status = _messageFor(error);
      _rollbackToConfirmed();
      return false;
    } on Exception {
      if (gen != _generation) return true;
      _status = watchlistSyncFailedMessage;
      _rollbackToConfirmed();
      return false;
    }
  }

  String _messageFor(WatchlistRequestException error) {
    if (error.statusCode == 429) return watchlistRateLimitedMessage;
    if (_isTooMany(error)) return watchlistCapMessage;
    final bad = _invalidSymbol(error);
    if (bad != null) return watchlistInvalidSymbolMessage(bad);
    return watchlistSyncFailedMessage;
  }

  bool _isTooMany(WatchlistRequestException error) {
    return error.statusCode == 400 &&
        error.message.contains('too many symbols');
  }

  String? _invalidSymbol(WatchlistRequestException error) {
    if (error.statusCode != 400 || _isTooMany(error)) return null;
    final match = _invalidSymbolPattern.firstMatch(error.message);
    if (match == null) return null;
    return match.group(1) ?? match.group(2);
  }

  void _adopt(Set<String> next) {
    _symbols = Set.of(next);
    _confirmed = Set.of(next);
    _serverKnown = true;
    _prefs?.favourites = _symbols;
    _prefs?.watchlistUpgradePending = false;
    _clearPendingFromStatus();
    _notify();
  }

  /// True when the set after the latest refetch, including symbols the
  /// server merged in, is the set the server returned.
  bool _editsOnServer() => _serverKnown && setEquals(_symbols, _confirmed);

  bool _isBlocked(List<String> payload) {
    final blocked = _blockedUpgrade;
    if (blocked == null) return false;
    return _sameSymbols(payload, blocked.toSet());
  }

  void _blockUpgrade(List<String> payload) {
    _blockedUpgrade = List.of(payload);
    _prefs?.watchlistBlockedUpgrade = _blockedUpgrade;
    _prefs?.watchlistUpgradePending = true;
  }

  void _clearBlockedUpgrade() {
    _blockedUpgrade = null;
    _prefs?.watchlistBlockedUpgrade = null;
    _prefs?.watchlistUpgradePending = false;
  }

  String? _upgradeStatus(List<String> dropped, List<String> skipped) {
    final leftOff = dropped.isEmpty ? null : watchlistLeftOffMessage(dropped);
    final invalid = skipped.isEmpty
        ? null
        : watchlistInvalidSymbolsMessage(skipped);
    if (leftOff == null) return invalid;
    if (invalid == null) return leftOff;
    return '$leftOff $invalid';
  }

  void _markPending() {
    final current = _status;
    if (current == null || current.isEmpty) {
      _status = watchlistPendingMessage;
    } else if (!current.contains(watchlistPendingMessage)) {
      _status = '$current $watchlistPendingMessage';
    }
    // Persisted only with a blocked upgrade body (_blockUpgrade). A
    // post-migration timeout stays in-memory so the next launch does
    // not announce stars a migrated fetch is about to drop.
    _notify();
  }

  void _clearPendingFromStatus() {
    final current = _status;
    if (current == null) {
      _prefs?.watchlistUpgradePending = false;
      return;
    }
    if (current == watchlistPendingMessage) {
      _status = null;
      _prefs?.watchlistUpgradePending = false;
      return;
    }
    if (!current.contains(watchlistPendingMessage)) {
      _prefs?.watchlistUpgradePending = false;
      return;
    }
    _status = current
        .replaceAll(watchlistPendingMessage, '')
        .replaceAll(RegExp(r'\s+'), ' ')
        .trim();
    if (_status!.isEmpty) _status = null;
    _prefs?.watchlistUpgradePending = false;
  }

  /// A migration upload was rejected. Pending adds and removals stay so
  /// the next fetch retries the same union. A symbol is removed only
  /// when the response names it as invalid.
  void _rejectPendingUpload(WatchlistRequestException? error) {
    _status = error == null ? watchlistSyncFailedMessage : _messageFor(error);
    _serverKnown = false;
    _notify();
  }

  void _rollbackToConfirmed() {
    _symbols = Set.of(_confirmed);
    _pendingAdds.clear();
    _pendingRemoves.clear();
    _prefs?.favourites = _symbols;
    _notify();
  }

  void _markMigrated() {
    _prefs?.watchlistMigrated = true;
  }

  Future<bool> _enqueueBool(Future<bool> Function() action) {
    final done = Completer<bool>();
    _tail = _chain(_tail, () async {
      try {
        final ok = await action();
        if (!done.isCompleted) done.complete(ok);
      } on Exception {
        if (!done.isCompleted) done.complete(false);
      } catch (error, stack) {
        if (!done.isCompleted) done.completeError(error, stack);
        rethrow;
      }
    });
    return done.future;
  }

  Future<void> _enqueueVoid(Future<void> Function() action) {
    final done = Completer<void>();
    _tail = _chain(_tail, () async {
      try {
        await action();
      } finally {
        if (!done.isCompleted) done.complete();
      }
    });
    return done.future;
  }

  void _notify() {
    if (_disposed) return;
    notifyListeners();
  }

  @override
  void dispose() {
    _disposed = true;
    super.dispose();
  }
}

class _CappedWatchlist {
  final List<String> kept;
  final List<String> dropped;

  const _CappedWatchlist(this.kept, this.dropped);
}

final _invalidSymbolPattern = RegExp(
  r'invalid symbol \\?"([^"\\]*)\\?"'
  r'|symbol too long \(max \d+ chars\): \\?"([^"\\]*)\\?"',
);

Future<void> _chain(Future<void> previous, Future<void> Function() next) {
  return previous.then(
    (_) => next(),
    onError: (Object _, StackTrace __) => next(),
  );
}
