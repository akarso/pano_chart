# Settings & Filters

## Sort Order

Change how tickers are ranked in the grid:

- **Sideways** — structural consolidation score (channel quality, oscillation, drift control)
- **Compression** — squeeze detection (width contraction, boundary convergence, directional pressure)
- **Breakout** — boundary violation strength with volume/volatility expansion
- **Trend** — linear regression R² combined with normalized slope
- **Volume** — 24h trading volume
- **Change** — percentage price change
- **Gain / Loss** — close-to-close return

The natural flow is **sideways → compression → breakout → trend**, so sorting by sideways finds tokens consolidating, compression finds those about to move, and breakout catches active moves.

## Timeframes

Switch between **1m, 5m, 15m, 1h, 4h, and 1D** candle intervals. The scoring algorithm adapts to each timeframe, and the volatility activity overlay switches accordingly (minute-of-day patterns for intraday, day-of-week patterns for 1D).

## Columns

Toggle which data columns appear in the grid rows. Options include sparkline, price, score values, and percentage change.

## Sparkline Mode

Choose between **normalized** sparklines (scaled to fit each row uniformly) and **hi-res** sparklines (preserving relative price scale).

## Stablecoin Filter

Toggle stablecoin pairs on or off. When disabled, USDT/USDC/DAI-type pairs are hidden from the grid.

## Favourites

Long-press any ticker to star it. Starred tokens can be filtered to the top of the list for quick monitoring.

## Chart Indicators

In the detail view, tap the settings icon to toggle:

- **RSI** — relative strength index oscillator
- **EMA Clouds** — exponential moving average bands
- **ATR** — average true range volatility
- **Volatility Activity** — historical activity heatmap overlay

## Exchange Selection

Choose your preferred exchange for the one-tap trade buttons on the detail screen. The symbol is pre-loaded so you can go from chart to order in seconds.
