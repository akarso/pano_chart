# Market Intelligence

Pano Charts goes beyond individual token analysis with a suite of market-wide intelligence tools.

## Market Regime (headline)

The Market Pulse headline classifies the overall crypto market by **scoring the merged market tape** — the same Trend / Sideways / Compression / Breakout calculators used on one chart, applied to a composite series of all tracked tokens.

- **Compression** — volatility contracting on the tape, the market is coiling
- **Sideways** — the tape is range-bound with low directional conviction
- **Trend** — the tape shows sustained directional structure
- **Expansion** — the tape shows breakout / volatility expansion

**Tape confidence** is how strongly that dominant structure wins on the composite — not “how many tokens agree.”

## Token Participation (metrics)

Separately, **participation** shows how individual tokens’ score mixes distribute across the four regimes. High trend participation means many tokens look structurally trendy; it can disagree with the headline when the tape is grinding up but most charts are choppy.

Use participation to understand how widely structure is shared across tokens, not as a substitute for “is the market going up?”

## Composite Price Index

Two normalized indexes (rebased to 100 at the first bar):

1. **Equal weight (median)** — resists crypto outliers; a single 50% mover won’t skew the reading.
2. **Volume weighted** — quote-volume-weighted mean; closer to money-flow / a heavier tape.

The headline regime prefers the volume-weighted series when available.

## Transition Probabilities

A heuristic model that estimates the likelihood of the market shifting from the current regime to another. The key signal is **breakout pressure**:

> breakout pressure = compression participation × (1 + volatility slope) × regime age factor

When many tokens are compressed, volatility is expanding, and the current regime has lasted a while, transition probability rises.

## Regime History

A timeline of past regime periods with start dates, durations, and transitions. Useful for understanding how long regimes typically last and recognizing when the current one is overdue for a shift.

## How to Use This

1. Check the **headline regime** — what does the merged tape look like?
2. Check the **composite chart** (volume-weighted vs median) — does price path agree?
3. Check **participation** — is the move broad or concentrated?
4. Check **transition probabilities** — is a shift likely soon?
5. If compression participation is high and breakout pressure is building, sort the grid by **breakout**
6. If the tape is trending, sort by **trend** to find the strongest individual movers
