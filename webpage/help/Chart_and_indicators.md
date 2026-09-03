# Chart & Indicators

## Interactive Chart

The detail screen features a full candlestick chart with touch controls:

- **Pinch** to zoom in/out on the time axis
- **Drag** to pan through history
- **Tap and hold** to activate the crosshair with exact price and time readout

The chart auto-loads candle data for the selected timeframe and symbol. With Pro, it refreshes live at intervals matched to the timeframe (10s for 1m, up to 1h for 1D).

## Indicator Panels

Toggle indicators from the settings icon in the chart toolbar:

### RSI (Relative Strength Index)

Classic momentum oscillator rendered below the candles. Shows overbought (>70) and oversold (<30) zones.

### EMA Clouds

Exponential moving average bands overlaid on candles. The cloud fill between fast and slow EMAs gives a visual sense of trend direction and strength.

### ATR (Average True Range)

Volatility indicator shown as a separate panel. Useful for gauging how "noisy" a token is and setting appropriate stop-loss distances.

## Volatility Activity Overlay

A color-coded bar panel showing **historical activity patterns** derived from 150 days of 1-minute candle data:

- On **intraday timeframes** (1m through 4h), each bar represents a time-of-day window — showing which hours tend to be most volatile
- On the **1D timeframe**, bars represent days of the week — showing which weekdays are historically most active

### Adaptive Coloring

Colors adjust dynamically as you zoom and pan:

- **Green** — below-average activity for the visible range
- **Yellow** — moderate activity
- **Red** — high activity (historically spike-prone)

The thresholds are based on percentiles of the visible data, so the color distribution always shows relative contrast regardless of zoom level.

## Macro Event Markers

Economic calendar events appear as vertical markers on the chart:

- **Past events** — solid markers showing what already happened
- **Future events** — dashed markers projecting upcoming events onto the time axis

The projection window is timeframe-dependent (e.g. 24h ahead on 1h charts, 2 days ahead on 4h/1D). This lets you see at a glance if a major event is approaching.

## Volume Bars

Standard volume bars are always visible below the candle area, colored by candle direction (green for up, red for down).
