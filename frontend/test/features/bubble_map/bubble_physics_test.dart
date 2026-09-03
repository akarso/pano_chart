import 'dart:math' as math;

import 'package:flutter_test/flutter_test.dart';

import 'package:pano_chart_frontend/features/bubble_map/bubble_physics.dart';

void main() {
  late BubblePhysics physics;

  setUp(() {
    physics = BubblePhysics();
  });

  group('BubblePhysics wall collisions', () {
    test('body bounces off left wall', () {
      final body = PhysicsBody(x: 5, y: 100, radius: 20, vx: -200, vy: 0);
      physics.step([body], dt: 1 / 60, width: 400, height: 400);

      // After step, body should be pushed to at least x = radius.
      expect(body.x, greaterThanOrEqualTo(body.radius));
      // Velocity should have reversed direction (positive).
      expect(body.vx, greaterThan(0));
    });

    test('body bounces off right wall', () {
      final body = PhysicsBody(x: 395, y: 100, radius: 20, vx: 200, vy: 0);
      physics.step([body], dt: 1 / 60, width: 400, height: 400);

      expect(body.x, lessThanOrEqualTo(400 - body.radius));
      expect(body.vx, lessThan(0));
    });

    test('body bounces off top wall', () {
      final body = PhysicsBody(x: 100, y: 5, radius: 20, vx: 0, vy: -200);
      physics.step([body], dt: 1 / 60, width: 400, height: 400);

      expect(body.y, greaterThanOrEqualTo(body.radius));
      expect(body.vy, greaterThan(0));
    });

    test('body bounces off bottom wall', () {
      final body = PhysicsBody(x: 100, y: 395, radius: 20, vx: 0, vy: 200);
      physics.step([body], dt: 1 / 60, width: 400, height: 400);

      expect(body.y, lessThanOrEqualTo(400 - body.radius));
      expect(body.vy, lessThan(0));
    });

    test('wall bounce adds angular velocity', () {
      final body = PhysicsBody(
        x: 5, y: 100, radius: 20,
        vx: -200, vy: 100,
        angularVelocity: 0,
      );
      physics.step([body], dt: 1 / 60, width: 400, height: 400);

      // Friction with wall should induce rotation.
      expect(body.angularVelocity, isNot(0));
    });
  });

  group('BubblePhysics circle-circle collisions', () {
    test('two overlapping bodies are separated', () {
      // Place two overlapping bodies.
      final a = PhysicsBody(x: 100, y: 100, radius: 30, vx: 50, vy: 0);
      final b = PhysicsBody(x: 140, y: 100, radius: 30, vx: -50, vy: 0);

      physics.step([a, b], dt: 1 / 60, width: 400, height: 400);

      final dx = b.x - a.x;
      final dy = b.y - a.y;
      final dist = math.sqrt(dx * dx + dy * dy);
      // After resolution, distance should be >= sum of radii.
      expect(dist, greaterThanOrEqualTo(a.radius + b.radius - 1));
    });

    test('head-on collision reverses velocities', () {
      final a = PhysicsBody(x: 100, y: 100, radius: 20, vx: 100, vy: 0);
      final b = PhysicsBody(x: 135, y: 100, radius: 20, vx: -100, vy: 0);

      physics.step([a, b], dt: 1 / 60, width: 400, height: 400);

      // After collision, a should be moving left, b moving right.
      expect(a.vx, lessThan(0));
      expect(b.vx, greaterThan(0));
    });

    test('collision conserves momentum approximately', () {
      final a = PhysicsBody(x: 100, y: 200, radius: 30, vx: 80, vy: 20);
      final b = PhysicsBody(x: 150, y: 200, radius: 20, vx: -30, vy: -10);

      final pxBefore = a.mass * a.vx + b.mass * b.vx;
      final pyBefore = a.mass * a.vy + b.mass * b.vy;

      physics.step([a, b], dt: 1 / 60, width: 800, height: 800);

      final pxAfter = a.mass * a.vx + b.mass * b.vx;
      final pyAfter = a.mass * a.vy + b.mass * b.vy;

      // Momentum should be approximately conserved (within drag tolerance).
      expect(pxAfter, closeTo(pxBefore, pxBefore.abs() * 0.1));
      expect(pyAfter, closeTo(pyBefore, pyBefore.abs() * 0.1 + 5));
    });

    test('collision induces angular velocity on both bodies', () {
      // Glancing collision — offset vertically so tangential component exists.
      final a = PhysicsBody(x: 100, y: 95, radius: 20, vx: 100, vy: 0);
      final b = PhysicsBody(x: 135, y: 105, radius: 20, vx: -100, vy: 0);

      physics.step([a, b], dt: 1 / 60, width: 400, height: 400);

      // Both should have picked up some spin.
      expect(a.angularVelocity, isNot(0));
      expect(b.angularVelocity, isNot(0));
    });

    test('non-overlapping bodies do not interact', () {
      final a = PhysicsBody(x: 50, y: 200, radius: 20, vx: 0, vy: 0);
      final b = PhysicsBody(x: 300, y: 200, radius: 20, vx: 0, vy: 0);

      physics.step([a, b], dt: 1 / 60, width: 400, height: 400);

      expect(a.vx, equals(0));
      expect(b.vx, equals(0));
    });
  });

  group('BubblePhysics drag and rest', () {
    test('linear drag slows bodies over time', () {
      final body = PhysicsBody(x: 200, y: 200, radius: 20, vx: 100, vy: 0);

      for (var i = 0; i < 60; i++) {
        physics.step([body], dt: 1 / 60, width: 400, height: 400);
      }

      // After ~1 second of drag, velocity should be significantly reduced.
      expect(body.vx.abs(), lessThan(50));
    });

    test('angular drag slows rotation over time', () {
      final body = PhysicsBody(
        x: 200, y: 200, radius: 20,
        angularVelocity: 5.0,
      );

      for (var i = 0; i < 120; i++) {
        physics.step([body], dt: 1 / 60, width: 400, height: 400);
      }

      expect(body.angularVelocity.abs(), lessThan(1.0));
    });

    test('isAtRest returns true for stationary bodies', () {
      final bodies = [
        PhysicsBody(x: 100, y: 100, radius: 20, vx: 0, vy: 0),
        PhysicsBody(x: 200, y: 200, radius: 20, vx: 0, vy: 0),
      ];

      expect(physics.isAtRest(bodies), isTrue);
    });

    test('isAtRest returns false for moving bodies', () {
      final bodies = [
        PhysicsBody(x: 100, y: 100, radius: 20, vx: 100, vy: 0),
      ];

      expect(physics.isAtRest(bodies), isFalse);
    });

    test('isAtRest returns false for spinning bodies', () {
      final bodies = [
        PhysicsBody(x: 100, y: 100, radius: 20, angularVelocity: 1.0),
      ];

      expect(physics.isAtRest(bodies), isFalse);
    });

    test('bodies eventually settle to rest without gravity', () {
      final bodies = [
        PhysicsBody(x: 100, y: 200, radius: 25, vx: 80, vy: -60),
        PhysicsBody(x: 250, y: 150, radius: 20, vx: -40, vy: 30),
        PhysicsBody(x: 160, y: 300, radius: 30, vx: 20, vy: -80),
      ];

      // Run for 10 simulated seconds.
      for (var i = 0; i < 600; i++) {
        physics.step(bodies, dt: 1 / 60, width: 400, height: 400);
      }

      expect(physics.isAtRest(bodies), isTrue);
    });
  });

  group('BubblePhysics gravity', () {
    test('gravity accelerates bodies downward', () {
      final body = PhysicsBody(x: 200, y: 100, radius: 20, vx: 0, vy: 0);

      // Apply downward gravity for a few frames.
      for (var i = 0; i < 10; i++) {
        physics.step([body],
            dt: 1 / 60, width: 400, height: 400, gravityY: 500);
      }

      expect(body.vy, greaterThan(0));
      expect(body.y, greaterThan(100));
    });

    test('sideways gravity pushes bodies laterally', () {
      final body = PhysicsBody(x: 200, y: 200, radius: 20, vx: 0, vy: 0);

      for (var i = 0; i < 10; i++) {
        physics.step([body],
            dt: 1 / 60, width: 400, height: 400, gravityX: 300);
      }

      expect(body.vx, greaterThan(0));
    });
  });

  group('BubblePhysics mass', () {
    test('mass is proportional to radius squared', () {
      final small = PhysicsBody(x: 0, y: 0, radius: 10);
      final big = PhysicsBody(x: 0, y: 0, radius: 20);

      expect(big.mass, equals(4 * small.mass));
    });

    test('heavy body barely moves in collision with light body', () {
      // Large body (r=60) hit by small body (r=10).
      final heavy = PhysicsBody(x: 200, y: 200, radius: 60, vx: 0, vy: 0);
      final light = PhysicsBody(x: 265, y: 200, radius: 10, vx: -200, vy: 0);

      physics.step([heavy, light], dt: 1 / 60, width: 600, height: 600);

      // Heavy body should barely budge.
      expect(heavy.vx.abs(), lessThan(20));
      // Light body should bounce away fast.
      expect(light.vx.abs(), greaterThan(50));
    });
  });

  group('BubblePhysics edge cases', () {
    test('empty bodies list does not throw', () {
      expect(
        () => physics.step([], dt: 1 / 60, width: 400, height: 400),
        returnsNormally,
      );
    });

    test('single body with no velocity stays put', () {
      final body = PhysicsBody(x: 200, y: 200, radius: 20);
      physics.step([body], dt: 1 / 60, width: 400, height: 400);

      expect(body.x, closeTo(200, 1));
      expect(body.y, closeTo(200, 1));
    });

    test('large dt is handled without explosion', () {
      final body = PhysicsBody(x: 200, y: 200, radius: 20, vx: 1000, vy: 1000);
      // Even with a huge dt, the body should stay within bounds.
      physics.step([body], dt: 1.0, width: 400, height: 400);

      expect(body.x, inInclusiveRange(body.radius, 400 - body.radius));
      expect(body.y, inInclusiveRange(body.radius, 400 - body.radius));
    });

    test('bodies stay within viewport after many steps with gravity', () {
      final bodies = List.generate(10, (i) => PhysicsBody(
        x: 40.0 + i * 35,
        y: 200,
        radius: 15,
        vx: (i.isEven ? 50 : -50).toDouble(),
        vy: -30,
      ));

      for (var i = 0; i < 300; i++) {
        physics.step(bodies,
            dt: 1 / 60, width: 400, height: 400, gravityY: 300);
      }

      for (final b in bodies) {
        expect(b.x, inInclusiveRange(0, 400));
        expect(b.y, inInclusiveRange(0, 400));
      }
    });
  });
}
