# Market Intelligence

Pano Charts goes beyond individual token analysis with a suite of market-wide intelligence tools.

## Market State & Breadth

The market state panel classifies the overall crypto market into one of four regimes:

- **Compression** — volatility contracting across many tokens, the market is coiling
- **Sideways** — range-bound with low directional conviction
- **Trend** — sustained directional movement with broad participation
- **Expansion** — high volatility with wide swings in both directions

Each state comes with a **confidence score** and a **breadth breakdown** showing what percentage of the 150 tracked tokens agree with the classification. High breadth = strong signal.

## Composite Price Index

A normalized median price index computed across all tracked tokens. The median (rather than mean) is used because it resists distortion from crypto outliers — a single 50% mover won't skew the reading.

The index is displayed as a timeline, giving you a "market pulse" view of how the aggregate is moving.

## Regime Detection

A single-pass aggregator that combines three signals:

1. **Volatility expansion** — ratio of short-term to long-term ATR
2. **Dispersion** — mean absolute deviation of returns across tokens
3. **Breadth** — compression and trend participation rates

The combination determines the current regime and how close the market is to a transition point.

## Transition Probabilities

A heuristic model that estimates the likelihood of the market shifting from the current regime to another. The key signal is **breakout pressure**:

> breakout pressure = compression breadth × (1 + volatility slope) × regime age factor

When many tokens are compressed, volatility is expanding, and the current regime has lasted a while, transition probability rises. This helps you prepare for the next phase rather than react to it.

## Regime History

A timeline of past regime periods with start dates, durations, and transitions. Useful for understanding how long regimes typically last and recognizing when the current one is overdue for a shift.

## How to Use This

The natural workflow:

1. Check **market state** — is the market in compression, sideways, trend, or expansion?
2. Check **transition probabilities** — is a shift likely soon?
3. If compression is high and breakout pressure is building, sort the grid by **breakout** to find tokens leading the move
4. If the market is trending, sort by **trend** to find the strongest movers
5. Use **fragility scores** on individual tokens to avoid crowded positions that might reverse
6. Check the **confidence dot** on setup quality — green means the context supports the setup; red means proceed with caution
7. Look at **breakout bars** with their confidence dots — a high breakout score with a red confidence dot is less trustworthy than a moderate score with a green dot
