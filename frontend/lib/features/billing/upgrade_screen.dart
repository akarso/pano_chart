import 'package:flutter/material.dart';
import 'package:in_app_purchase/in_app_purchase.dart';
import 'package:url_launcher/url_launcher.dart';
import 'billing_manager.dart';
import 'solana_payment_screen.dart';

/// Full-screen dialog for upgrading to Pro or managing an existing
/// subscription.
///
/// * Shows product price and a Subscribe button when not subscribed.
/// * Shows "Manage subscription" link when already subscribed.
/// * Supports "Restore purchases" for returning users.
class UpgradeScreen extends StatefulWidget {
  final BillingManager billingManager;

  const UpgradeScreen({Key? key, required this.billingManager})
      : super(key: key);

  @override
  State<UpgradeScreen> createState() => _UpgradeScreenState();
}

class _UpgradeScreenState extends State<UpgradeScreen> {
  late final BillingManager _billing;

  @override
  void initState() {
    super.initState();
    _billing = widget.billingManager;
    _billing.onChanged = () {
      if (mounted) setState(() {});
    };
  }

  @override
  void dispose() {
    _billing.onChanged = null;
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final isActive = _billing.status.active;
    final product = _billing.product;
    final busy = _billing.busy;

    return Scaffold(
      backgroundColor: Colors.black,
      appBar: AppBar(
        title: Text(isActive ? 'Manage Subscription' : 'Upgrade to Pro'),
        backgroundColor: Colors.black,
        foregroundColor: Colors.white,
        elevation: 0,
      ),
      body: SafeArea(
        child: Center(
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 32),
            child: isActive
                ? _buildActiveContent()
                : _buildUpgradeContent(product, busy),
          ),
        ),
      ),
    );
  }

  Widget _buildActiveContent() {
    final expiresAt = _billing.status.expiresAt;
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        const Icon(Icons.check_circle, color: Color(0xFF00E6C0), size: 64),
        const SizedBox(height: 16),
        const Text(
          'You are subscribed to Pro',
          style: TextStyle(
              fontSize: 20,
              fontWeight: FontWeight.bold,
              color: Colors.white),
          textAlign: TextAlign.center,
        ),
        if (expiresAt != null) ...[
          const SizedBox(height: 8),
          Text(
            'Expires: ${_formatDate(expiresAt)}',
            style: const TextStyle(color: Colors.white54, fontSize: 14),
          ),
        ],
        const SizedBox(height: 32),
        OutlinedButton(
          onPressed: _openSubscriptionManagement,
          style: OutlinedButton.styleFrom(
            foregroundColor: Colors.white,
            side: const BorderSide(color: Colors.white24),
            padding: const EdgeInsets.symmetric(horizontal: 32, vertical: 14),
          ),
          child: const Text('Manage subscription'),
        ),
      ],
    );
  }

  Widget _buildUpgradeContent(ProductDetails? product, bool busy) {
    final price = product?.price ?? '\$4.99 / month';
    final trialDays = _billing.trialDaysRemaining;
    final trialExpired = !_billing.hasFullAccess && !_billing.status.active;
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(
          trialExpired ? Icons.lock_outline : Icons.workspace_premium,
          color: const Color(0xFF00E6C0),
          size: 64,
        ),
        const SizedBox(height: 16),
        Text(
          trialExpired ? 'Trial Expired' : 'Upgrade to Pro',
          style: const TextStyle(
            fontSize: 24,
            fontWeight: FontWeight.bold,
            color: Colors.white,
          ),
          textAlign: TextAlign.center,
        ),
        if (trialDays > 0 && !_billing.status.active) ...[
          const SizedBox(height: 8),
          Text(
            '$trialDays day${trialDays == 1 ? '' : 's'} left in free trial',
            style: const TextStyle(color: Color(0xFF00E6C0), fontSize: 14),
          ),
        ],
        if (trialExpired) ...[
          const SizedBox(height: 8),
          const Text(
            'Your 14-day free trial has ended.\nSubscribe to keep using all features.',
            style: TextStyle(color: Colors.white54, fontSize: 14),
            textAlign: TextAlign.center,
          ),
        ],
        const SizedBox(height: 8),
        Text(
          price,
          style: const TextStyle(
            fontSize: 18,
            color: Color(0xFF00E6C0),
            fontWeight: FontWeight.w600,
          ),
        ),
        const SizedBox(height: 24),
        const Text(
          'Unlock all features with a monthly subscription.',
          style: TextStyle(color: Colors.white54, fontSize: 14),
          textAlign: TextAlign.center,
        ),
        const SizedBox(height: 32),
        SizedBox(
          width: double.infinity,
          child: ElevatedButton(
            onPressed: busy ? null : _onSubscribe,
            style: ElevatedButton.styleFrom(
              backgroundColor: const Color(0xFF00E6C0),
              foregroundColor: Colors.black,
              padding: const EdgeInsets.symmetric(vertical: 16),
              shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(12)),
              textStyle:
                  const TextStyle(fontSize: 16, fontWeight: FontWeight.w700),
            ),
            child: busy
                ? const SizedBox(
                    width: 20,
                    height: 20,
                    child: CircularProgressIndicator(
                        strokeWidth: 2, color: Colors.black),
                  )
                : const Text('Subscribe'),
          ),
        ),
        const SizedBox(height: 16),
        TextButton(
          onPressed: busy ? null : _onRestore,
          child: const Text(
            'Restore purchases',
            style: TextStyle(color: Colors.white54, fontSize: 14),
          ),
        ),
        const SizedBox(height: 8),
        const Divider(color: Colors.white12),
        const SizedBox(height: 8),
        SizedBox(
          width: double.infinity,
          child: OutlinedButton.icon(
            onPressed: busy ? null : _onPayWithSolana,
            icon: const Icon(Icons.currency_exchange, size: 18),
            label: const Text('Pay with Solana'),
            style: OutlinedButton.styleFrom(
              foregroundColor: const Color(0xFF00E6C0),
              side: const BorderSide(color: Color(0xFF00E6C0)),
              padding: const EdgeInsets.symmetric(vertical: 14),
              shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(12)),
              textStyle:
                  const TextStyle(fontSize: 15, fontWeight: FontWeight.w600),
            ),
          ),
        ),
      ],
    );
  }

  Future<void> _onSubscribe() async {
    final started = await _billing.purchase();
    if (!started && mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Could not start purchase flow')),
      );
    }
  }

  Future<void> _onRestore() async {
    await _billing.restorePurchases();
    if (mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(_billing.status.active
              ? 'Subscription restored!'
              : 'No previous subscription found'),
        ),
      );
    }
  }

  void _openSubscriptionManagement() {
    launchUrl(
      Uri.parse(
          'https://play.google.com/store/account/subscriptions'),
      mode: LaunchMode.externalApplication,
    );
  }

  Future<void> _onPayWithSolana() async {
    final result = await Navigator.of(context).push<bool>(
      MaterialPageRoute(
        builder: (_) => SolanaPaymentScreen(billingManager: _billing),
      ),
    );
    if (result == true && mounted) {
      setState(() {}); // refresh UI with new subscription status
    }
  }

  String _formatDate(DateTime dt) {
    return '${dt.year}-${dt.month.toString().padLeft(2, '0')}-${dt.day.toString().padLeft(2, '0')}';
  }
}
