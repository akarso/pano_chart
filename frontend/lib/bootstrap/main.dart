import 'package:firebase_core/firebase_core.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import '../core/app_lifecycle_manager.dart';
import '../core/config/config.dart';
import '../core/di/di.dart';
import '../core/di/composition_root.dart';
import '../features/billing/billing_manager.dart';
import '../features/billing/trial_manager.dart';
import '../features/overview/overview_widget.dart';
import '../features/social/notification_service.dart';
import '../features/social/social_feed_view_model.dart';
import '../infrastructure/preferences_service.dart';
import '../infrastructure/stablecoin_config.dart';

/// Exposed bootstrap function so tests can instantiate the app without side effects.
Widget bootstrapApp({
  required AppConfig config,
  PreferencesService? prefs,
  StablecoinConfig stablecoins = const StablecoinConfig({}),
  BillingManager? billingManager,
  SocialFeedViewModel? socialFeedViewModel,
  AppLifecycleManager? lifecycleManager,
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
  final setupApi = root.createSetupApi();
  final fragilityApi = root.createFragilityApi();
  final behaviorApi = root.createBehaviorApi();
  final volatilityApi = root.createVolatilityApi();
  final socialVm = socialFeedViewModel;
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
      setupApi: setupApi,
      fragilityApi: fragilityApi,
      behaviorApi: behaviorApi,
      volatilityApi: volatilityApi,
      socialFeedViewModel: socialVm,
    ),
  );
  final app = component.createApp();
  if (lifecycleManager != null) {
    return AppLifecycleScope(manager: lifecycleManager, child: app);
  }
  return app;
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
  final socialRoot = CompositionRoot(apiBaseUrl: config.apiBaseUrl);
  final socialFeedViewModel =
      socialRoot.createSocialFeedViewModel(userId: prefs.userId);
  socialFeedViewModel.attachPrefs(prefs);

  // ── Firebase + device registration + local notifications ──
  final notificationService = NotificationService();
  try {
    await Firebase.initializeApp();
    await notificationService.init();

    // Request notification permission (required on Android 13+ / iOS).
    await FirebaseMessaging.instance.requestPermission();

    final fcmToken = await FirebaseMessaging.instance.getToken();
    if (fcmToken != null) {
      final deviceApi = socialRoot.createDeviceRegistrationApi();
      final platform =
          defaultTargetPlatform == TargetPlatform.iOS ? 'ios' : 'android';
      try {
        await deviceApi.register(
          userId: prefs.userId,
          deviceId: prefs.userId,
          fcmToken: fcmToken,
          platform: platform,
        );
      } catch (_) {
        // Non-fatal — push won't work but app is still usable.
      }

      // Re-register whenever the FCM token refreshes.
      FirebaseMessaging.instance.onTokenRefresh.listen((newToken) {
        deviceApi.register(
          userId: prefs.userId,
          deviceId: prefs.userId,
          fcmToken: newToken,
          platform: platform,
        );
      });
    }
  } catch (_) {
    // Firebase unavailable (e.g. web, desktop, emulator without config).
  }

  // Wire local notification display for new social posts.
  socialFeedViewModel.onNewPost = notificationService.showNewPostNotification;

  final lifecycleManager = AppLifecycleManager()..init();

  runApp(bootstrapApp(
    config: config,
    prefs: prefs,
    stablecoins: stablecoins,
    billingManager: billingManager,
    socialFeedViewModel: socialFeedViewModel,
    lifecycleManager: lifecycleManager,
  ));
}
