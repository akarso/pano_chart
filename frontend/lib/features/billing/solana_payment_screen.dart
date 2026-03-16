import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'api/sol_price_info.dart';
import 'billing_manager.dart';

/// Screen guiding the user through a Solana payment:
///
/// 1. Fetches the current SOL price and required amount from the backend.
/// 2. Displays the merchant wallet address (tap to copy).
/// 3. Provides a text field for the user to paste their TX signature.
/// 4. Verifies the payment server-side and activates the subscription.
class SolanaPaymentScreen extends StatefulWidget {
  final BillingManager billingManager;

  const SolanaPaymentScreen({Key? key, required this.billingManager})
      : super(key: key);

  @override
  State<SolanaPaymentScreen> createState() => _SolanaPaymentScreenState();
}

class _SolanaPaymentScreenState extends State<SolanaPaymentScreen> {
  final _txController = TextEditingController();
  SolPriceInfo? _priceInfo;
  bool _loading = true;
  bool _verifying = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _fetchPrice();
  }

  @override
  void dispose() {
    _txController.dispose();
    super.dispose();
  }

  Future<void> _fetchPrice() async {
    try {
      final info = await widget.billingManager.getSolPrice();
      if (mounted) {
        setState(() {
          _priceInfo = info;
          _loading = false;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _error = 'Could not fetch SOL price. Please try again later.';
          _loading = false;
        });
      }
    }
  }

  Future<void> _verify() async {
    final sig = _txController.text.trim();
    if (sig.isEmpty) {
      setState(() => _error = 'Please enter your transaction signature.');
      return;
    }

    setState(() {
      _verifying = true;
      _error = null;
    });

    final success = await widget.billingManager.verifySolanaPayment(sig);

    if (!mounted) return;

    if (success) {
      Navigator.of(context).pop(true);
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Payment verified! Subscription active.')),
      );
    } else {
      setState(() {
        _verifying = false;
        _error =
            'Verification failed. Make sure the TX is confirmed and the '
            'correct amount was sent to the wallet shown above.';
      });
    }
  }

  void _copyWallet() {
    if (_priceInfo == null) return;
    Clipboard.setData(ClipboardData(text: _priceInfo!.wallet));
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(content: Text('Wallet address copied to clipboard')),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.black,
      appBar: AppBar(
        title: const Text('Pay with Solana'),
        backgroundColor: Colors.black,
        foregroundColor: Colors.white,
        elevation: 0,
      ),
      body: SafeArea(
        child: _loading
            ? const Center(child: CircularProgressIndicator())
            : SingleChildScrollView(
                padding: const EdgeInsets.symmetric(horizontal: 24),
                child: _buildContent(),
              ),
      ),
    );
  }

  Widget _buildContent() {
    if (_priceInfo == null) {
      return Center(
        child: Text(
          _error ?? 'Unable to load pricing.',
          style: const TextStyle(color: Colors.redAccent, fontSize: 16),
          textAlign: TextAlign.center,
        ),
      );
    }

    final info = _priceInfo!;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const SizedBox(height: 24),
        // Price info
        Center(
          child: Column(
            children: [
              const Icon(Icons.currency_exchange,
                  color: Color(0xFF00E6C0), size: 56),
              const SizedBox(height: 12),
              Text(
                '\$${info.priceUSD.toStringAsFixed(2)} / month',
                style: const TextStyle(
                  fontSize: 22,
                  fontWeight: FontWeight.bold,
                  color: Colors.white,
                ),
              ),
              const SizedBox(height: 4),
              Text(
                '≈ ${info.requiredSOL.toStringAsFixed(6)} SOL',
                style: const TextStyle(
                  fontSize: 18,
                  color: Color(0xFF00E6C0),
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(height: 4),
              Text(
                '1 SOL ≈ \$${info.solPrice.toStringAsFixed(2)}',
                style: const TextStyle(color: Colors.white38, fontSize: 13),
              ),
            ],
          ),
        ),
        const SizedBox(height: 32),

        // Instructions
        const Text(
          'How to pay',
          style: TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.w600,
            color: Colors.white,
          ),
        ),
        const SizedBox(height: 8),
        _stepText('1. Send the exact SOL amount above to the wallet below.'),
        _stepText('2. Wait for the transaction to be confirmed.'),
        _stepText('3. Paste the transaction signature below and tap Verify.'),
        const SizedBox(height: 24),

        // Wallet address
        const Text(
          'Send SOL to:',
          style: TextStyle(color: Colors.white54, fontSize: 13),
        ),
        const SizedBox(height: 6),
        GestureDetector(
          onTap: _copyWallet,
          child: Container(
            width: double.infinity,
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: Colors.white.withValues(alpha: 0.05),
              borderRadius: BorderRadius.circular(8),
              border: Border.all(color: Colors.white12),
            ),
            child: Row(
              children: [
                Expanded(
                  child: SelectableText(
                    info.wallet,
                    style: const TextStyle(
                      color: Color(0xFF00E6C0),
                      fontSize: 13,
                      fontFamily: 'monospace',
                    ),
                  ),
                ),
                const SizedBox(width: 8),
                const Icon(Icons.copy, color: Colors.white54, size: 18),
              ],
            ),
          ),
        ),
        const SizedBox(height: 24),

        // TX signature input
        const Text(
          'Transaction Signature:',
          style: TextStyle(color: Colors.white54, fontSize: 13),
        ),
        const SizedBox(height: 6),
        TextField(
          controller: _txController,
          style: const TextStyle(color: Colors.white, fontFamily: 'monospace', fontSize: 13),
          decoration: InputDecoration(
            hintText: 'Paste your Solana TX signature here...',
            hintStyle: const TextStyle(color: Colors.white24),
            filled: true,
            fillColor: Colors.white.withValues(alpha: 0.05),
            border: OutlineInputBorder(
              borderRadius: BorderRadius.circular(8),
              borderSide: const BorderSide(color: Colors.white12),
            ),
            enabledBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(8),
              borderSide: const BorderSide(color: Colors.white12),
            ),
            focusedBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(8),
              borderSide: const BorderSide(color: Color(0xFF00E6C0)),
            ),
          ),
        ),
        const SizedBox(height: 24),

        // Verify button
        SizedBox(
          width: double.infinity,
          child: ElevatedButton(
            onPressed: _verifying ? null : _verify,
            style: ElevatedButton.styleFrom(
              backgroundColor: const Color(0xFF00E6C0),
              foregroundColor: Colors.black,
              padding: const EdgeInsets.symmetric(vertical: 16),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(12),
              ),
              textStyle:
                  const TextStyle(fontSize: 16, fontWeight: FontWeight.w700),
            ),
            child: _verifying
                ? const SizedBox(
                    width: 20,
                    height: 20,
                    child: CircularProgressIndicator(
                        strokeWidth: 2, color: Colors.black),
                  )
                : const Text('Verify Payment'),
          ),
        ),

        // Error message
        if (_error != null) ...[
          const SizedBox(height: 16),
          Text(
            _error!,
            style: const TextStyle(color: Colors.redAccent, fontSize: 13),
            textAlign: TextAlign.center,
          ),
        ],
        const SizedBox(height: 32),
      ],
    );
  }

  Widget _stepText(String text) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 4),
      child: Text(
        text,
        style: const TextStyle(color: Colors.white54, fontSize: 13),
      ),
    );
  }
}
