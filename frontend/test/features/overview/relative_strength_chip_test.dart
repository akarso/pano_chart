import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/overview/relative_strength_chip.dart';

void main() {
  test('RelativeStrengthChip_labelFor formats percent', () {
    expect(RelativeStrengthChip.labelFor(0.032), '+3.2% vs mkt');
    expect(RelativeStrengthChip.labelFor(-0.011), '-1.1% vs mkt');
    expect(RelativeStrengthChip.labelFor(0.0), '0.0% vs mkt');
  });

  test('RelativeStrengthChip_colorFor matches rounded label', () {
    expect(RelativeStrengthChip.colorFor(0.0004), Colors.grey);
    expect(RelativeStrengthChip.colorFor(-0.0004), Colors.grey);
    expect(RelativeStrengthChip.colorFor(0.032), Colors.green);
    expect(RelativeStrengthChip.colorFor(-0.011), Colors.red);
  });

  testWidgets('RelativeStrengthChip_hiddenWhenRsNull', (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: RelativeStrengthChip(rsAvailable: true, rs: null),
        ),
      ),
    );
    expect(find.byKey(const ValueKey('relative-strength-chip')), findsNothing);
  });

  testWidgets('RelativeStrengthChip_hiddenWhenRsUnavailable', (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: RelativeStrengthChip(rsAvailable: false, rs: 0.05),
        ),
      ),
    );
    expect(find.byKey(const ValueKey('relative-strength-chip')), findsNothing);
  });

  testWidgets('RelativeStrengthChip_showsZeroAndOpensDialog', (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: RelativeStrengthChip(
            rsAvailable: true,
            rs: 0.0,
            beta: 1.25,
          ),
        ),
      ),
    );
    expect(find.text('0.0% vs mkt'), findsOneWidget);
    await tester.tap(find.byKey(const ValueKey('relative-strength-chip')));
    await tester.pumpAndSettle();
    expect(find.textContaining('Relative strength: this symbol'), findsOneWidget);
    expect(find.textContaining('Beta for this symbol: 1.25'), findsOneWidget);
  });
}
