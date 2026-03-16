import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import '../core/config/config.dart';
import '../core/di/di.dart';
import '../core/di/composition_root.dart';
import '../features/billing/billing_manager.dart';
import '../features/billing/trial_manager.dart';
import '../features/overview/overview_widget.dart';
import '../infrastructure/preferences_service.dart';
import '../infrastructure/stablecoin_config.dart';

/// Exposed bootstrap function so tests can instantiate the app without side effects.
Widget bootstrapApp({
  required AppConfig config,
  PreferencesService? prefs,
  StablecoinConfig stablecoins = const StablecoinConfig({}),
  BillingManager? billingManager,
}) {
  final root = CompositionRoot(
    apiBaseUrl: config.apiBaseUrl,
    stablecoinPadding: stablecoins.count,
  );
  final overviewViewModel = root.createOverviewViewModel();
  final getCandleSeries = root.createGetCandleSeries();
  final eventsViewModel = root.createEventsViewModel();
  final bubbleMapViewModel = root.createBubbleMapViewModel();
  final fearGreedApi = root.createFearGreedApi();
  final marketStateApi = root.createMarketStateApi();
  final compositeIndexApi = root.createCompositeIndexApi();
  final regimeApi = root.createRegimeApi();
  final transitionApi = root.createTransitionApi();
  final regimeHistoryApi = root.createRegimeHistoryApi();
  final newsViewModel = root.createNewsViewModel();
  final component = AppComponent(
    config,
    home: OverviewWidget(
      viewModel: overviewViewModel,
      getCandleSeries: getCandleSeries,
      eventsViewModel: eventsViewModel,
      prefs: prefs,
      bubbleMapViewModel: bubbleMapViewModel,
      fearGreedApi: fearGreedApi,
      marketStateApi: marketStateApi,
      compositeIndexApi: compositeIndexApi,
      regimeApi: regimeApi,
      transitionApi: transitionApi,
      regimeHistoryApi: regimeHistoryApi,
      stablecoins: stablecoins,
      newsViewModel: newsViewModel,
      billingManager: billingManager,
    ),
  );
  return component.createApp();
}

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  SystemChrome.setEnabledSystemUIMode(SystemUiMode.edgeToEdge);
  SystemChrome.setSystemUIOverlayStyle(const SystemUiOverlayStyle(
    statusBarColor: Color(0x00000000),
    systemNavigationBarColor: Color(0x00000000),
    systemNavigationBarDividerColor: Color(0x00000000),
  ));

  final prefs = await PreferencesService.create();
  final stablecoins = await loadStablecoinConfig();

  // Billing is only available on Android.
  BillingManager? billingManager;
  if (defaultTargetPlatform == TargetPlatform.android) {
    const config = AppConfig(
        apiBaseUrl: 'http://srv1024540.hstgr.cloud:8080', flavor: 'dev');
    final root = CompositionRoot(apiBaseUrl: config.apiBaseUrl);
    final trialManager = TrialManager(prefs.sharedPreferences);
    billingManager = root.createBillingManager(
      userId: prefs.userId,
      trialManager: trialManager,
    );
    await billingManager.init();
  }

  const config = AppConfig(
      apiBaseUrl: 'http://srv1024540.hstgr.cloud:8080', flavor: 'dev');
  runApp(bootstrapApp(
    config: config,
    prefs: prefs,
    stablecoins: stablecoins,
    billingManager: billingManager,
  ));
}
