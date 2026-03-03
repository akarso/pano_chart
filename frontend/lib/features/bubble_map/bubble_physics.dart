import 'dart:math' as math;

/// Mutable physics body for a single bubble.
///
/// Tracks position, velocity, rotation, angular velocity.
/// Mass is proportional to area (π r²).
class PhysicsBody {
  double x;
  double y;
  final double radius;

  double vx;
  double vy;
  double angle; // radians
  double angularVelocity; // rad/s

  /// Mass proportional to area.  The constant factor is irrelevant since
  /// we only need mass *ratios* in collision responses.
  double get mass => radius * radius;

  PhysicsBody({
    required this.x,
    required this.y,
    required this.radius,
    this.vx = 0.0,
    this.vy = 0.0,
    this.angle = 0.0,
    this.angularVelocity = 0.0,
  });
}

/// Deterministic 2-D rigid-body simulation for circular bodies.
///
/// Features:
/// * circle-circle elastic collisions (restitution coefficient)
/// * circle-wall elastic collisions (4 walls)
/// * linear drag (friction)
/// * angular drag (rotation friction)
/// * external gravity (e.g. from accelerometer)
///
/// The simulation is stateless between steps — all state lives in
/// [PhysicsBody] objects, making it easy to test.
class BubblePhysics {
  /// Coefficient of restitution for collisions (0 = perfectly inelastic,
  /// 1 = perfectly elastic).  0.82 gives a satisfying bouncy feel.
  static const double restitution = 0.82;

  /// Linear drag coefficient.  Applied as `v *= (1 - drag)` per step.
  static const double linearDrag = 0.015;

  /// Angular drag coefficient.  Applied as `ω *= (1 - angularDrag)`.
  /// Higher value damps rotation quickly once collisions stop.
  static const double angularDrag = 0.08;

  /// Friction coefficient used to convert tangential collision impulse
  /// into angular velocity change.  Kept low to avoid perpetual spin.
  static const double frictionCoeff = 0.12;

  /// Number of constraint-solving iterations per step.  Multiple passes
  /// prevent bubbles from overlapping when many pile up together.
  static const int _solverIterations = 3;

  /// Minimum gap between bodies after collision resolution (prevents
  /// bodies from overlapping on the next frame).
  static const double separationEpsilon = 0.5;

  /// Velocity magnitude below which a body is considered at rest.
  static const double restThreshold = 0.5;

  /// angular velocity below which the body is considered non-spinning.
  static const double angularRestThreshold = 0.01;

  /// Advance the simulation by [dt] seconds.
  ///
  /// [bodies] — mutable list of physics bodies.
  /// [width], [height] — bounding box (walls at 0/width and 0/height).
  /// [gravityX], [gravityY] — external acceleration (pixels/s²),
  ///   typically from the accelerometer.
  void step(
    List<PhysicsBody> bodies, {
    required double dt,
    required double width,
    required double height,
    double gravityX = 0.0,
    double gravityY = 0.0,
  }) {
    // 1. Apply gravity + integrate velocity → position.
    for (final b in bodies) {
      b.vx += gravityX * dt;
      b.vy += gravityY * dt;

      // Linear drag.
      b.vx *= (1 - linearDrag);
      b.vy *= (1 - linearDrag);

      b.x += b.vx * dt;
      b.y += b.vy * dt;

      // Angular drag.
      b.angularVelocity *= (1 - angularDrag);
      b.angle += b.angularVelocity * dt;

      // Snap to rest when very slow.
      if (b.vx.abs() < restThreshold && b.vy.abs() < restThreshold) {
        // Only zero-out if gravity is small (phone flat on table).
        if (gravityX.abs() < 20 && gravityY.abs() < 20) {
          if (b.vx.abs() < restThreshold) b.vx = 0;
          if (b.vy.abs() < restThreshold) b.vy = 0;
        }
      }
      if (b.angularVelocity.abs() < angularRestThreshold) {
        b.angularVelocity = 0;
      }
    }

    // 2. Multiple solver passes to resolve overlaps properly.
    for (var iter = 0; iter < _solverIterations; iter++) {
      // Walls first.
      for (final b in bodies) {
        _resolveWalls(b, width, height);
      }

      // Circle-circle collisions (O(n²) — fine for ≤50 bodies).
      for (var i = 0; i < bodies.length; i++) {
        for (var j = i + 1; j < bodies.length; j++) {
          _resolveCollision(bodies[i], bodies[j]);
        }
      }
    }

    // Final wall clamp — ensure nothing leaked past boundaries.
    for (final b in bodies) {
      _resolveWalls(b, width, height);
    }
  }

  /// Returns true when all bodies have come to rest.
  bool isAtRest(List<PhysicsBody> bodies) {
    for (final b in bodies) {
      if (b.vx.abs() > restThreshold ||
          b.vy.abs() > restThreshold ||
          b.angularVelocity.abs() > angularRestThreshold) {
        return false;
      }
    }
    return true;
  }

  // ---- wall collisions ----

  void _resolveWalls(PhysicsBody b, double width, double height) {
    // Left wall
    if (b.x - b.radius < 0) {
      b.x = b.radius;
      b.vx = b.vx.abs() * restitution;
      // Wall friction → spin.
      b.angularVelocity += frictionCoeff * b.vy / b.radius;
    }
    // Right wall
    if (b.x + b.radius > width) {
      b.x = width - b.radius;
      b.vx = -b.vx.abs() * restitution;
      b.angularVelocity -= frictionCoeff * b.vy / b.radius;
    }
    // Top wall
    if (b.y - b.radius < 0) {
      b.y = b.radius;
      b.vy = b.vy.abs() * restitution;
      b.angularVelocity -= frictionCoeff * b.vx / b.radius;
    }
    // Bottom wall
    if (b.y + b.radius > height) {
      b.y = height - b.radius;
      b.vy = -b.vy.abs() * restitution;
      b.angularVelocity += frictionCoeff * b.vx / b.radius;
    }
  }

  // ---- circle-circle collisions ----

  void _resolveCollision(PhysicsBody a, PhysicsBody b) {
    final dx = b.x - a.x;
    final dy = b.y - a.y;
    final distSq = dx * dx + dy * dy;
    final minDist = a.radius + b.radius;

    if (distSq >= minDist * minDist || distSq == 0) return;

    final dist = math.sqrt(distSq);
    // Normal from a → b.
    final nx = dx / dist;
    final ny = dy / dist;

    // Separate overlapping bodies.
    final overlap = minDist - dist + separationEpsilon;
    final totalMass = a.mass + b.mass;
    a.x -= nx * overlap * (b.mass / totalMass);
    a.y -= ny * overlap * (b.mass / totalMass);
    b.x += nx * overlap * (a.mass / totalMass);
    b.y += ny * overlap * (a.mass / totalMass);

    // Relative velocity along collision normal.
    final dvx = a.vx - b.vx;
    final dvy = a.vy - b.vy;
    final dvn = dvx * nx + dvy * ny;

    // Only resolve if bodies are approaching.
    if (dvn <= 0) return;

    // Impulse magnitude (1-D elastic collision with restitution).
    final j = dvn * (1 + restitution) / (1 / a.mass + 1 / b.mass);

    a.vx -= j * nx / a.mass;
    a.vy -= j * ny / a.mass;
    b.vx += j * nx / b.mass;
    b.vy += j * ny / b.mass;

    // Tangential component → angular velocity (surface friction).
    final tx = -ny; // tangent perpendicular to normal
    final ty = nx;
    final dvt = dvx * tx + dvy * ty;

    a.angularVelocity += frictionCoeff * dvt / a.radius;
    b.angularVelocity -= frictionCoeff * dvt / b.radius;
  }
}
