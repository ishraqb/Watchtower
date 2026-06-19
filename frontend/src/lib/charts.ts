import {
	createChart,
	LineSeries,
	type IChartApi,
	type ISeriesApi,
	type UTCTimestamp
} from 'lightweight-charts';

export interface SymbolChart {
	chart: IChartApi;
	series: ISeriesApi<'Line'>;
	lastTime: number;
}

const chartOptions = {
	layout: {
		background: { color: 'transparent' },
		textColor: '#7a7a7a',
		fontFamily: 'Inter, sans-serif',
		attributionLogo: false
	},
	// Gridless, Robinhood-style canvas.
	grid: {
		vertLines: { visible: false },
		horzLines: { visible: false }
	},
	rightPriceScale: { visible: false },
	leftPriceScale: { visible: false },
	timeScale: {
		borderVisible: false,
		timeVisible: true,
		secondsVisible: false
	},
	crosshair: {
		vertLine: { color: 'rgba(255,255,255,0.2)', labelVisible: false, width: 1 as const },
		horzLine: { visible: false, labelVisible: false }
	},
	handleScale: false,
	handleScroll: false,
	autoSize: true
};

/** Creates a line chart bound to a container element. */
export function createSymbolChart(container: HTMLElement): SymbolChart {
	const chart = createChart(container, chartOptions);
	const series = chart.addSeries(LineSeries, {
		color: '#00c805',
		lineWidth: 2,
		priceLineVisible: false,
		lastValueVisible: false,
		crosshairMarkerVisible: true,
		crosshairMarkerRadius: 4
	});
	return { chart, series, lastTime: 0 };
}

/**
 * Pushes a price point onto the series. Lightweight Charts requires strictly
 * increasing timestamps, so duplicate-second ticks are nudged forward by 1s.
 */
export function pushPrice(sc: SymbolChart, epochMillis: number, price: number): void {
	let time = Math.floor(epochMillis / 1000);
	if (time <= sc.lastTime) {
		time = sc.lastTime + 1;
	}
	sc.lastTime = time;
	sc.series.update({ time: time as UTCTimestamp, value: price });
}

/**
 * Replaces the entire series with a historical set of (unix-second, price)
 * points. Used when switching the time-range tab. Colors the line green/red
 * based on whether the series ends above or below where it started.
 */
export function setSeriesData(
	sc: SymbolChart,
	points: { time: number; value: number }[],
	baseline?: number
): void {
	const data = points
		.filter((p) => Number.isFinite(p.time) && Number.isFinite(p.value))
		.map((p) => ({ time: p.time as UTCTimestamp, value: p.value }));

	if (data.length > 0) {
		// Color against the same baseline as the displayed % (prior close for 1D,
		// otherwise the first point) so the line and the percentage always agree.
		const base = baseline && baseline > 0 ? baseline : data[0].value;
		const up = data[data.length - 1].value >= base;
		sc.series.applyOptions({ color: up ? '#00c805' : '#ff5000' });
		sc.lastTime = data[data.length - 1].time as number;
	}

	sc.series.setData(data);
	sc.chart.timeScale().fitContent();
}
