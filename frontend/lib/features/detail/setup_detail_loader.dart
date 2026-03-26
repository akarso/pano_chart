import 'package:flutter/material.dart';

import '../../domain/symbol.dart';
import '../../domain/timeframe.dart';
import '../candles/application/get_candle_series.dart';
import '../volatility/http_volatility_api.dart';
import 'chart_navigation.dart';
import 'detail_screen.dart';
import 'http_behavior_api.dart';
import 'http_fragility_api.dart';
import 'http_setup_api.dart';

/// Loads candle data for [symbol] then pushes [DetailScreen].
///
/// Used by [NotificationRouter] for setup-of-the-day notifications where
/// only the symbol string is available.
class SetupDetailLoader extends StatefulWidget {
  final String symbol;
  final String timeframe;
  final GetCandleSeries getCandleSeries;
  final SetupApi? setupApi;
  final FragilityApi? fragilityApi;
  final BehaviorApi? behaviorApi;
  final VolatilityApi? volatilityApi;
  final bool isProUser;

  const SetupDetailLoader({
    Key? key,
    required this.symbol,
    this.timeframe = '4h',
    required this.getCandleSeries,
    this.setupApi,
    this.fragilityApi,
    this.behaviorApi,
    this.volatilityApi,
    this.isProUser = false,
  }) : super(key: key);

  @override
  State<SetupDetailLoader> createState() => _SetupDetailLoaderState();
}

class _SetupDetailLoaderState extends State<SetupDetailLoader> {
  String? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final input = buildDetailChartInput(
        symbol: widget.symbol,
        timeframe: widget.timeframe,
      );
      final series = await widget.getCandleSeries.execute(input);
      if (!mounted) return;
      // Replace this loader screen with the real detail screen.
      Navigator.of(context).pushReplacement(
        MaterialPageRoute(
          builder: (_) => DetailScreen(
            symbol: AppSymbol(widget.symbol),
            timeframe: Timeframe(widget.timeframe),
            series: series,
            warmupCount: kIndicatorWarmup,
            initialVisibleCount: kSparklineCandles,
            getCandleSeries: widget.getCandleSeries,
            setupApi: widget.setupApi,
            fragilityApi: widget.fragilityApi,
            behaviorApi: widget.behaviorApi,
            volatilityApi: widget.volatilityApi,
            isProUser: widget.isProUser,
          ),
        ),
      );
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e.toString();
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFF121212),
      appBar: AppBar(title: Text(widget.symbol)),
      body: Center(
        child: _error != null
            ? Text(_error!, style: const TextStyle(color: Colors.red))
            : const CircularProgressIndicator(),
      ),
    );
  }
}
